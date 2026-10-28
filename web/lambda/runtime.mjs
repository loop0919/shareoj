// Lambda Runtime API client for the provided.al2023 runtime. It loads the function's handler
// setting (_HANDLER, such as lambda/index.handler) and calls it as the Node.js runtime did.
import { existsSync } from 'node:fs'
import { format } from 'node:util'

const api = `http://${process.env.AWS_LAMBDA_RUNTIME_API}/2018-06-01/runtime`
const root = process.env.LAMBDA_TASK_ROOT ?? process.cwd()
let requestId

// Write lines as the Node.js runtime does: time, request ID, and level, with newlines turned
// into \r so that one call stays one CloudWatch event.
for (const [method, level] of [['debug', 'DEBUG'], ['info', 'INFO'], ['log', 'INFO'], ['warn', 'WARN'], ['error', 'ERROR']]) {
  console[method] = (...args) => {
    process.stdout.write(`${new Date().toISOString()}\t${requestId ?? 'undefined'}\t${level}\t${format(...args).replaceAll('\n', '\r')}\n`)
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

function errorBody(error) {
  return JSON.stringify({
    errorType: error?.name ?? 'Error',
    errorMessage: String(error?.message ?? error),
    stackTrace: String(error?.stack ?? '').split('\n').slice(1).map(line => line.trim()),
  })
}

async function loadHandler() {
  const setting = /^(.+)\.([^./]+)$/.exec(process.env._HANDLER ?? '')
  if (!setting) throw new Error(`Invalid handler setting: ${process.env._HANDLER}`)
  const file = ['.mjs', '.js'].map(extension => `${root}/${setting[1]}${extension}`).find(path => existsSync(path))
  if (!file) throw new Error(`Cannot find the handler module ${setting[1]} in ${root}`)
  const handler = (await import(file))[setting[2]]
  if (typeof handler !== 'function') throw new TypeError(`${process.env._HANDLER} is not a function`)
  return handler
}

let handler
try {
  handler = await loadHandler()
} catch (error) {
  console.error(error)
  await post('init/error', errorBody(error), 'Runtime.ImportModuleError').catch(console.error)
  process.exit(1)
}

for (;;) {
  let invocation
  try {
    invocation = await fetch(`${api}/invocation/next`)
    if (!invocation.ok) throw new Error(`Runtime API answered ${invocation.status} for the next invocation`)
  } catch (error) {
    // A dropped connection loses no invocation that this runtime has accepted; ask again.
    console.error(error)
    await new Promise(resolve => setTimeout(resolve, 100))
    continue
  }
  requestId = invocation.headers.get('lambda-runtime-aws-request-id')
  const deadline = Number(invocation.headers.get('lambda-runtime-deadline-ms'))
  const trace = invocation.headers.get('lambda-runtime-trace-id')
  if (trace) process.env._X_AMZN_TRACE_ID = trace
  else delete process.env._X_AMZN_TRACE_ID
  const context = {
    awsRequestId: requestId,
    invokedFunctionArn: invocation.headers.get('lambda-runtime-invoked-function-arn'),
    functionName: process.env.AWS_LAMBDA_FUNCTION_NAME,
    functionVersion: process.env.AWS_LAMBDA_FUNCTION_VERSION,
    memoryLimitInMB: process.env.AWS_LAMBDA_FUNCTION_MEMORY_SIZE,
    logGroupName: process.env.AWS_LAMBDA_LOG_GROUP_NAME,
    logStreamName: process.env.AWS_LAMBDA_LOG_STREAM_NAME,
    callbackWaitsForEmptyEventLoop: true,
    getRemainingTimeInMillis: () => Math.max(0, deadline - Date.now()),
  }
  const invocationPath = `invocation/${encodeURIComponent(requestId)}`
  try {
    const result = await handler(await invocation.json(), context)
    await post(`${invocationPath}/response`, JSON.stringify(result ?? null))
  } catch (error) {
    // Also reached when Lambda refuses the response, such as one over 6 MB.
    console.error(error)
    await post(`${invocationPath}/error`, errorBody(error), 'Unhandled').catch(console.error)
  }
  requestId = undefined
}
