"""Operator-owned runtime commands. Never accept executable paths from a job."""
ROOT = '/opt/judge-runtimes'
GCC = ROOT + '/gcc/bin/'
CLANG = ROOT + '/llvm/bin/'
INCLUDES = ['-I' + ROOT + '/include']
LIBS = ['-Wl,-rpath,' + ROOT + '/gcc/lib64']
JAVA_CP = ROOT + '/java-libs/*'


def native(compiler, standard, source, extra=()):
    return dict(source=source, compile=[compiler, '-std=' + standard, '-O2', '-pipe',
                '/box/' + source, *extra, '-o', '/box/main'], run=['/box/main'], artifact='native')


def python(executable):
    return dict(source='main.py', compile=[executable, '-I', '-B', '-c',
                "import ast; ast.parse(open('/box/main.py').read())"],
                run=[executable, '-I', '-B', '/box/main'], artifact='source')


RUNTIMES = {
    'cpp17-isolate': native('/usr/bin/g++', 'c++17', 'main.cpp'),
    'c23-gcc-isolate': native(GCC + 'gcc', 'c23', 'main.c', ['-lm', *LIBS]),
    'c23-clang-isolate': native(CLANG + 'clang', 'c23', 'main.c', ['--gcc-toolchain=' + ROOT + '/gcc', '-lm', *LIBS]),
    'cpp23-gcc-isolate': native(GCC + 'g++', 'c++23', 'main.cpp', [*INCLUDES, *LIBS]),
    'cpp23-clang-isolate': native(CLANG + 'clang++', 'c++23', 'main.cpp', ['--gcc-toolchain=' + ROOT + '/gcc', '-stdlib=libstdc++', *INCLUDES, *LIBS]),
    'python314-isolate': python(ROOT + '/python/bin/python3.14'),
    'pypy311-isolate': python(ROOT + '/pypy/bin/pypy3'),
    'codon020-isolate': dict(source='main.py', compile=[ROOT + '/codon/bin/codon', 'build', '-release', '/box/main.py', '-o', '/box/main'], run=['/box/main'], artifact='native'),
    'rust2024-isolate': dict(source='main.rs', compile=[ROOT + '/rust/bin/rustc', '--edition=2024', '-C', 'opt-level=2', '-C', 'linker=/usr/bin/gcc', '@/opt/judge-runtimes/rust/extern.args', '/box/main.rs', '-o', '/box/main'], run=['/box/main'], artifact='native'),
    'java24-isolate': dict(source='Main.java', compile=[ROOT + '/java/bin/javac', '-J-Xmx512m', '-J-XX:ActiveProcessorCount=1', '-J-XX:-UsePerfData', '--release', '24', '-encoding', 'UTF-8', '-cp', JAVA_CP, '-d', '/box/classes', '/box/Main.java'],
        run=[ROOT + '/java/bin/java', '-Xms16m', '-Xmx256m', '-Xss1m', '-XX:MaxMetaspaceSize=96m', '-XX:ReservedCodeCacheSize=32m', '-XX:MaxDirectMemorySize=32m', '-XX:+UseSerialGC', '-XX:ActiveProcessorCount=1', '-XX:+ExitOnOutOfMemoryError', '-XX:-UsePerfData', '-cp', '/box/main:' + JAVA_CP, 'Main'], artifact='java'),
}

RUNTIMES['java25-isolate'] = dict(RUNTIMES['java24-isolate'],
    compile=[arg.replace(ROOT + '/java/', ROOT + '/java25/').replace(JAVA_CP, ROOT + '/java-libs/ac_library.jar')
             if arg != '24' else '25' for arg in RUNTIMES['java24-isolate']['compile']],
    run=[arg.replace(ROOT + '/java/', ROOT + '/java25/').replace(JAVA_CP, ROOT + '/java-libs/ac_library.jar')
         for arg in RUNTIMES['java24-isolate']['run']])
RUNTIMES['csharp14-isolate'] = dict(source='main.cs',
    compile=[ROOT + '/dotnet/dotnet', ROOT + '/csharp-tools/csc.dll', '@' + ROOT + '/csharp-tools/compile.args'],
    run=[ROOT + '/dotnet/dotnet', '/box/main.dll'], artifact='dotnet',
    files=['main.runtimeconfig.json', 'main.deps.json', 'MathNet.Numerics.dll', 'ac-library-csharp.dll'],
    env=['DOTNET_CLI_TELEMETRY_OPTOUT=1', 'DOTNET_EnableDiagnostics=0', 'DOTNET_gcServer=0', 'DOTNET_PROCESSOR_COUNT=1'])
RUNTIMES['go127-isolate'] = dict(source='main.go',
    compile=[ROOT + '/go/bin/go', '-C', '/box', 'build', '-buildmode=exe', '-mod=vendor', '-p=1', '-trimpath', '-buildvcs=false', '-o', '/box/main', '/box/main.go'],
    run=['/box/main'], artifact='native',
    env=['GOTOOLCHAIN=local', 'GOPROXY=off', 'GOSUMDB=off', 'GOWORK=off', 'CGO_ENABLED=0', 'GOMAXPROCS=1', 'GOCACHE=/box/go-cache', 'GOTMPDIR=/box'])
RUNTIMES['nim22-isolate'] = dict(source='main.nim',
    compile=[ROOT + '/nim/bin/nim', 'cpp', '-d:release', '--opt:speed', '--mm:refc', '--parallelBuild:1', '--hints:off',
             '--skipUserCfg:on', '--skipParentCfg:on', '--skipProjCfg:on', '--noNimblePath', '--nimcache:/box/nimcache', '-o:/box/main', '/box/main.nim'],
    run=['/box/main'], artifact='native',
    env=['LD_LIBRARY_PATH=' + ROOT + '/nim-deps/lib:' + ROOT + '/gcc/lib64'])

# Module resolution requires the original suffix; dependencies are operator-owned mounts.
NODE = ROOT + '/node/bin/node'
NATIVE_ENV = ['LD_LIBRARY_PATH=' + ROOT + '/addition-native/lib:' + ROOT + '/ruby/lib',
              'GEOS_LIBRARY_PATH=' + ROOT + '/addition-native/lib/libgeos_c.so']
RUNTIMES['haskell-ghc910-isolate'] = dict(source='Main.hs',
    compile=[ROOT + '/ghc/bin/ghc', '-O2', '-threaded', '-rtsopts', '-j1',
             '-L' + ROOT + '/addition-native/lib',
             '@' + ROOT + '/haskell-deps/compile.args', '/box/Main.hs', '-o', '/box/main'],
    run=['/box/main', '+RTS', '-N1', '-RTS'], artifact='native',
    env=[*NATIVE_ENV, 'PATH=' + ROOT + '/haskell-deps/bin:/usr/bin:/bin'])
RUNTIMES['ruby40-isolate'] = dict(source='main.rb', program='main.rb', artifact='source',
    compile=[ROOT + '/ruby/bin/ruby', '-c', '/box/main.rb'],
    run=[ROOT + '/ruby/bin/ruby', '-rbundler/setup', '/box/main.rb'],
    env=[*NATIVE_ENV, 'BUNDLE_GEMFILE=' + ROOT + '/ruby-deps/Gemfile',
         'BUNDLE_PATH=' + ROOT + '/ruby-deps/bundle', 'BUNDLE_FROZEN=true'])
RUNTIMES['ruby-truffle40-isolate'] = dict(source='main.rb', program='main.rb', artifact='source',
    compile=[ROOT + '/truffleruby/bin/ruby', '-c', '/box/main.rb'],
    run=[ROOT + '/truffleruby/bin/ruby', '--engine.Mode=latency', '/box/main.rb'],
    # Every installed gem version is locked at build time; avoid loading Bundler on each run.
    env=[*NATIVE_ENV, 'GEM_HOME=' + ROOT + '/truffleruby-deps/bundle/truffleruby/40.0.0.1',
         'GEM_PATH=' + ROOT + '/truffleruby-deps/bundle/truffleruby/40.0.0.1'])

for engine, executable in [('node24', NODE), ('bun14', ROOT + '/bun/bun'), ('deno29', ROOT + '/deno/deno')]:
    deno = engine == 'deno29'
    env = ['NO_COLOR=1', 'UV_THREADPOOL_SIZE=1']
    if deno:
        env += ['DENO_NO_UPDATE_CHECK=1', 'DENO_DIR=/box/deno-cache']
    if engine == 'bun14':
        env += ['BUN_INSTALL_CACHE_DIR=/box/bun-cache', 'BUN_RUNTIME_TRANSPILER_CACHE_PATH=0',
                'BUN_CONFIG_NO_CLEAR_TERMINAL=1']
    for language, extension in [('javascript', 'js'), ('typescript', 'ts')]:
        source = 'main.' + extension
        if deno:
            command = [executable, 'run', '--no-check', '--cached-only', '--no-prompt',
                       '--allow-read=/box,' + ROOT, '--allow-write=/box', '--allow-env',
                       '--config=' + ROOT + '/deno-deps/deno.json', '/box/' + source]
        elif engine == 'bun14':
            command = [executable, '--no-install', '/box/' + source]
        else:
            command = [executable, '/box/main.js']
        compile = [NODE, '--check', '/box/' + source]
        artifact, output, program = 'source', source, source
        if language == 'typescript':
            if deno:
                compile = [executable, 'check', '--cached-only',
                           '--config=' + ROOT + '/deno-deps/deno.json', '/box/main.ts']
            else:
                compile = [NODE, ROOT + '/js-deps/node_modules/typescript/bin/tsc',
                           '/box/main.ts', '--target', 'ES2022', '--module', 'NodeNext',
                           '--moduleResolution', 'NodeNext', '--skipLibCheck', '--noEmitOnError',
                           '--pretty', 'false', '--outDir', '/box/compiled',
                           '--types', 'node' if engine == 'node24' else 'bun']
                artifact, output, program = 'generated-source', 'compiled/main.js', 'main.js'
                command = [executable, *(['--no-install'] if engine == 'bun14' else []), '/box/main.js']
        RUNTIMES[language + '-' + engine + '-isolate'] = dict(source=source, program=program,
            compile=compile, run=command, artifact=artifact, compile_output=output,
            node_modules='js-deps/node_modules', deno_cache=deno, env=env)

ENVIRONMENT = ['LANG=C.UTF-8', 'LC_ALL=C.UTF-8', 'HOME=/box',
               'OPENBLAS_NUM_THREADS=1', 'OMP_NUM_THREADS=1', 'MKL_NUM_THREADS=1',
               'NUMEXPR_NUM_THREADS=1', 'PYTHONDONTWRITEBYTECODE=1',
               'CODON_PATH=' + ROOT + '/codon/lib/codon/stdlib']
