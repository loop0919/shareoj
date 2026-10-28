// Lambda Runtime API client for the provided.al2023 runtime. It loads the function's handler
// setting (_HANDLER, such as lambda/index.handler) and runs it as the Node.js runtime's own client
// does: the same log lines and error reports, and the same exits when an error escapes the handler.
import { existsSync } from 'node:fs'
import { format } from 'node:util'

// The handler may replace the global fetch; the Runtime API calls keep this one.
const fetch = globalThis.fetch
const api = `http://${process.env.AWS_LAMBDA_RUNTIME_API}/2018-06-01/runtime`
const root = process.env.LAMBDA_TASK_ROOT ?? process.cwd()
const requestIdKey = Symbol.for('aws.lambda.runtime.requestId')
const tenantIdKey = Symbol.for('aws.lambda.runtime.tenantId')

// Write lines as the Node.js runtime does: time, request ID, and level, with newlines turned
// into \r so that one call stays one CloudWatch event. The last request ID stays set afterwards.
for (const [method, level] of [['trace', 'TRACE'], ['debug', 'DEBUG'], ['info', 'INFO'], ['log', 'INFO'], ['warn', 'WARN'], ['error', 'ERROR'], ['fatal', 'FATAL']]) {
  console[method] = (...args) => {
    process.stdout.write(`${new Date().toISOString()}\t${globalThis[requestIdKey]}\t${level}\t${format(...args).replaceAll('\n', '\r')}\n`)
  }
}

// The Node.js runtime's error classes, named Runtime.<class>.
class RuntimeError extends Error {}
for (const name of ['ImportModuleError', 'HandlerNotFound', 'MalformedHandlerName', 'UserCodeSyntaxError', 'UnhandledPromiseRejection']) {
  RuntimeError[name] = class extends RuntimeError {}
  RuntimeError[name].prototype.name = `Runtime.${name}`
}

// What the Node.js runtime posts to the Runtime API for an error. DEL becomes %7F, so that the
// type also fits the error type header.
function report(error) {
  const escape = text => text?.replaceAll('\x7F', '%7F')
  if (error instanceof Error || (typeof error?.name === 'string' && typeof error?.message === 'string' && typeof error?.stack === 'string')) {
    return { errorType: escape(error.name), errorMessage: escape(error.message), trace: escape(String(error.stack)).split('\n') }
  }
  return { errorType: typeof error, errorMessage: String(error), trace: [] }
}

// What it logs for one: a tab, then the error as JSON with its own properties and stack lines.
function formatted(error) {
  const value = error instanceof Error ? error : new Error(error)
  try {
    return `\t${JSON.stringify(value, (_key, item) => item instanceof Error
      ? { errorType: item.name, errorMessage: item.message, code: item.code, ...item, ...(typeof item.stack === 'string' ? { stack: item.stack.split('\n') } : {}) }
      : item)}`
  } catch {
    return `\t${JSON.stringify(report(value))}`
  }
}

async function post(path, body, errorType) {
  const response = await fetch(`${api}/${path}`, {
    method: 'POST',
    body,
    headers: errorType ? { 'Lambda-Runtime-Function-Error-Type': errorType } : {},
  })
  if (!response.ok) throw new Error(`Runtime API refused ${path}: ${response.status}`)
}

function postError(path, error) {
  const body = report(error)
  return post(path, JSON.stringify(body), body.errorType)
}

// An error outside the handler ends the runtime, as in the Node.js runtime: it is reported for the
// invocation in progress (or as an initialization error before the first one), and Lambda starts
// a new runtime for the next invocation.
let current
let served = false
async function stop(message, error, code) {
  if (message) console.error(message, formatted(error))
  const path = current ? `invocation/${encodeURIComponent(current)}/error` : served ? undefined : 'init/error'
  if (path) await postError(path, error).catch(() => {})
  process.exit(code)
}
process.on('uncaughtException', error => stop('Uncaught Exception', error, 129))
process.on('unhandledRejection', (reason, promise) => {
  const error = new RuntimeError.UnhandledPromiseRejection(reason)
  Object.assign(error, { reason, promise })
  stop('Unhandled Promise Rejection', error, 128)
})

async function loadHandler() {
  const setting = /^([^.]*)\.(.*)$/.exec(process.env._HANDLER ?? '')
  if (!setting) throw new RuntimeError.MalformedHandlerName('Bad handler')
  const file = ['.mjs', '.js'].map(extension => `${root}/${setting[1]}${extension}`).find(path => existsSync(path))
  if (!file) throw new RuntimeError.ImportModuleError(`Error: Cannot find module '${setting[1]}'`)
  let module
  try {
    module = await import(file)
  } catch (error) {
    throw error instanceof SyntaxError ? new RuntimeError.UserCodeSyntaxError(String(error)) : error
  }
  const handler = setting[2].split('.').reduce((value, key) => value?.[key], module)
  if (!handler) throw new RuntimeError.HandlerNotFound(`${process.env._HANDLER} is undefined or not exported`)
  if (typeof handler !== 'function') throw new RuntimeError.HandlerNotFound(`${process.env._HANDLER} is not a function`)
  return handler
}

let handler
try {
  handler = await loadHandler()
} catch (error) {
  await stop('Uncaught Exception', error, 129)
}

const json = text => text === null ? undefined : JSON.parse(text)
for (;;) {
  let invocation
  try {
    invocation = await fetch(`${api}/invocation/next`)
    if (!invocation.ok) throw new Error(`Runtime API answered ${invocation.status} for the next invocation`)
  } catch (error) {
    console.log(`Unexpected Top Level Error: ${error}`)
    await stop(undefined, error, 129)
  }
  const headers = invocation.headers
  const requestId = headers.get('lambda-runtime-aws-request-id')
  const deadline = Number(headers.get('lambda-runtime-deadline-ms'))
  globalThis[requestIdKey] = requestId
  globalThis[tenantIdKey] = headers.get('lambda-runtime-aws-tenant-id') ?? undefined
  const trace = headers.get('lambda-runtime-trace-id')
  if (trace) process.env._X_AMZN_TRACE_ID = trace
  else delete process.env._X_AMZN_TRACE_ID
  current = requestId
  served = true
  const invocationPath = `invocation/${encodeURIComponent(requestId)}`
  try {
    const context = {
      callbackWaitsForEmptyEventLoop: true,
      functionVersion: process.env.AWS_LAMBDA_FUNCTION_VERSION,
      functionName: process.env.AWS_LAMBDA_FUNCTION_NAME,
      memoryLimitInMB: process.env.AWS_LAMBDA_FUNCTION_MEMORY_SIZE,
      logGroupName: process.env.AWS_LAMBDA_LOG_GROUP_NAME,
      logStreamName: process.env.AWS_LAMBDA_LOG_STREAM_NAME,
      clientContext: json(headers.get('lambda-runtime-client-context')),
      identity: json(headers.get('lambda-runtime-cognito-identity')),
      invokedFunctionArn: headers.get('lambda-runtime-invoked-function-arn'),
      awsRequestId: requestId,
      tenantId: globalThis[tenantIdKey],
      getRemainingTimeInMillis: () => deadline - Date.now(),
    }
    const result = await handler(await invocation.json(), context)
    await post(`${invocationPath}/response`, JSON.stringify(result ?? null))
  } catch (error) {
    // Also reached when Lambda refuses the response, such as one over 6 MB.
    console.error('Invoke Error', formatted(error))
    await postError(`${invocationPath}/error`, error).catch(console.error)
  }
  current = undefined
}
