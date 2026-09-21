#!/usr/bin/python3
"""Run on the target Lightsail host, with the queue worker stopped. No AWS access."""
from host import judge, prepare_cgroup, verify_assets, slot
import json
import os
from pathlib import Path
from runtimes import RUNTIMES
from interactive_smoke import failure_program

with slot():
    runtime = verify_assets(full=True)
    prepare_cgroup()
    programs = [
        ('AC', '#include <cstdio>\nint main(){puts("3");}'),
        ('WA', '#include <cstdio>\nint main(){puts("4");}'),
        ('CE', 'this does not compile'),
        ('TLE', 'int main(){for(;;){}}'),
        ('TLE', '#include <unistd.h>\nint main(){sleep(60);}'),
        ('MLE', '#include <cstdlib>\nint main(){for(;;){volatile char* p=(char*)malloc(16<<20);if(p)for(int i=0;i<(16<<20);i+=4096)p[i]=1;}}'),
        ('MLE', '#include <unistd.h>\n#include <sys/wait.h>\n#include <cstdlib>\nint main(){for(int n=0;n<3;n++)if(fork()==0){volatile char* p=(char*)malloc(300<<20);if(!p)return 2;for(int i=0;i<(300<<20);i+=4096)p[i]=1;sleep(10);return 0;}while(wait(nullptr)>0){}}'),
        ('AC', '#include <unistd.h>\n#include <cstdio>\nint main(){for(int i=0;i<200;i++){int p=fork();if(p<0){puts("3");return 0;}if(p==0){sleep(10);return 0;}}puts("4");}'),
        ('OLE', '#include <cstdio>\nint main(){for(;;)putchar(65);}'),
        ('AC', '#include <cstdio>\n#include <unistd.h>\nint main(){puts(access("/run/judge/meta",F_OK)==-1?"3":"4");}'),
        ('AC', '#include <unistd.h>\n#include <cstdio>\nint main(){puts(access("/root/.aws/credentials",F_OK)==-1 && access("/opt/judge/worker.env",F_OK)==-1 && access("/etc/shadow",F_OK)==-1?"3":"4");}'),
        ('AC', '#include <sys/socket.h>\n#include <arpa/inet.h>\n#include <cstdio>\nint main(){int s=socket(AF_INET,SOCK_STREAM,0);sockaddr_in a{};a.sin_family=AF_INET;a.sin_port=htons(80);inet_pton(AF_INET,"169.254.169.254",&a.sin_addr);puts(connect(s,(sockaddr*)&a,sizeof(a))==-1?"3":"4");}'),
        ('AC', '#include <cstdio>\nint main(){FILE* f=fopen("marker","r");puts(f?"4":"3");if(f)fclose(f);f=fopen("marker","w");if(f)fclose(f);}'),
    ]
    for expected, source in programs:
        job = dict(runtime='cpp17-isolate', runtimeDigest=runtime, source=source,
                   timeLimitMs=1000, memoryLimitMb=512, cases=[dict(name='first', input='', output='3'), dict(name='fresh', input='', output='3')])
        result = judge(job, runtime)
        assert result['verdict'] == expected, (expected, result)
        print(expected, 'OK', flush=True)
    # A finite allocation must fail below its size and succeed above it. An
    # unbounded allocator alone would also pass with the old fixed 512 MiB cap.
    allocate = '#include <cstdlib>\n#include <cstdio>\nint main(){volatile char* p=(char*)malloc(MEMORY<<20);if(!p)return 2;for(int i=0;i<(MEMORY<<20);i+=4096)p[i]=1;puts("3");}'
    for memory in (64, 315, 512):
        for size, expected in ((memory // 2, 'AC'), (memory + 16, 'MLE')):
            source = allocate.replace('MEMORY', str(size))
            job = dict(runtime='cpp17-isolate', runtimeDigest=runtime, source=source,
                       timeLimitMs=1000, memoryLimitMb=memory, cases=[dict(input='', output='3')])
            result = judge(job, runtime)
            assert result['verdict'] == expected, (memory, expected, result)
            print('memory', memory, expected, 'OK', flush=True)
    fixtures = json.loads(Path('/opt/judge/language-smoke.json').read_text())
    requested = os.environ.get('JUDGE_SMOKE_RUNTIMES', ','.join(RUNTIMES)).split(',')
    if not requested or any(name not in RUNTIMES for name in requested):
        raise ValueError('invalid smoke runtime selection')
    passed, failed = [], []
    for name in requested:
        for fixture in fixtures[name]:
            job = dict(runtime=name, runtimeDigest=runtime, source=fixture['source'],
                       timeLimitMs=fixture.get('timeLimitMs', 1000), memoryLimitMb=512,
                       cases=[dict(name='first', input=fixture.get('input', ''), output=fixture.get('output', '3')),
                              dict(name='fresh', input=fixture.get('input', ''), output=fixture.get('output', '3'))])
            result = judge(job, runtime)
            if result['verdict'] != fixture['verdict']:
                print(name, fixture['name'], 'FAILED', result, flush=True)
                failed.append(name)
                break
            print(name, fixture['name'], result['verdict'], 'OK', flush=True)
        else:
            # Compile and execute this runtime as a checker, independently of C++ submissions.
            fixture = next(f for f in fixtures[name] if f['verdict'] == 'AC')
            literal = json.dumps(fixture.get('input', ''))
            source = '#include <cstdio>\nint main(){fputs(' + literal + ',stdout);}'
            checker = dict(runtime=name.removesuffix('-isolate'), source=fixture['source'])
            job = dict(runtime='cpp17-isolate', runtimeDigest=runtime, source=source, checker=checker,
                       timeLimitMs=1000, memoryLimitMb=512,
                       cases=[dict(input='', output='different expected output')] * 2)
            result = judge(job, runtime)
            if result['verdict'] != 'AC':
                print(name, 'checker FAILED', result, flush=True)
                failed.append(name)
                continue
            assertion = failure_program(name)
            job['checker'] = dict(runtime=name.removesuffix('-isolate'), source=assertion)
            result = judge(job, runtime)
            if result['verdict'] != 'WA':
                print(name, 'checker assertion FAILED', result, flush=True)
                failed.append(name)
                continue
            outputs = []
            generated = judge(dict(runtime=name, runtimeDigest=runtime, source=fixture['source'],
                generate=True, generationPrefix='test-files/' + 'a' * 32 + '/11111111-1111-4111-8111-111111111111/generated/',
                timeLimitMs=5000, memoryLimitMb=512,
                cases=[dict(input=fixture.get('input', ''), output='')] * 2), runtime,
                save_output=lambda data: outputs.append(data) or {})
            if generated['verdict'] != 'AC' or len(outputs) != 2 or any(
                    output.decode().split() != fixture.get('output', '3').split() for output in outputs):
                print(name, 'generation FAILED', generated, flush=True)
                failed.append(name)
                continue
            print(name, 'generation OK', flush=True)
            if name == 'cpp17-isolate':
                checker_source = r'''#include <cassert>
#include <fstream>
#include <iostream>
#include <unistd.h>
int main(int argc,char** argv){
 assert(argc==5); int n,x; std::ifstream(argv[1])>>n; assert(std::cin>>x); assert(x>=0 && x<=n);
 assert(std::ifstream(argv[2]).peek()==EOF); assert(std::ifstream(argv[3]).peek()!=EOF);
 std::ofstream score(argv[4]); assert(score.good()); score<<1;
 assert(access("marker",F_OK)==-1); std::ofstream("marker")<<1;
 assert(access("/run/judge/main",F_OK)==-1); assert(access("/root/.aws/credentials",F_OK)==-1);
}'''
                for want, code, output in [('AC', checker_source, '7'), ('WA', checker_source, '99'),
                                           ('JE', 'invalid checker', '7'), ('JE', 'int main(){for(;;){}}', '7')]:
                    job.update(source='#include <cstdio>\nint main(){puts("' + output + '");}',
                               checker=dict(runtime='cpp17', source=code), cases=[dict(input='10', output='')] * 2)
                    result = judge(job, runtime)
                    if result['verdict'] != want:
                        print(name, 'checker', want, 'FAILED', result, flush=True)
                        failed.append(name)
                        break
                else:
                    passed.append(name)
                    print(name, 'checker OK', flush=True)
                continue
            passed.append(name)
            print(name, 'checker OK', flush=True)
    import interactive_smoke
    try:
        interactive_smoke.run(runtime, passed, fixtures, judge)
        import testlib_smoke
        testlib_smoke.run(runtime, passed, judge)
    except Exception as error:
        # An incomplete role/isolation test can never authorize publication.
        print('judge role smoke FAILED', repr(error), flush=True)
        failed.extend(passed)
        passed = []
    print(json.dumps({'runtimeDigest': runtime, 'passedRuntimes': passed, 'failedRuntimes': failed}), flush=True)
    if failed:
        raise SystemExit(1)
