"""Run under the production service limits, with the worker stopped."""
import json
from pathlib import Path


def failure_program(name):
    if name.startswith(('cpp', 'c23')):
        return '#include <assert.h>\nint main(){assert(0);}'
    if name.startswith(('javascript-', 'typescript-')):
        return 'throw new Error("rejected");'
    families = {
        'clay20250308-isolate': '{abort();}',
        'dc152-isolate': '!kill -TERM $PPID\n',
        'nibbles101-isolate': 'error "rejected"',
        'brainfuck-tritium-isolate': '+[]',
        'whitespace13-isolate': ' \n\n\n\n\n',
        'rust2024-isolate': 'fn main(){assert!(false);}',
        'java25-isolate': 'public class Main { public static void main(String[] args){assert false;} }',
        'csharp14-isolate': 'class Program { static void Main(){throw new System.Exception();} }',
        'go127-isolate': 'package main\nfunc main(){panic("rejected")}',
        'nim22-isolate': 'doAssert false',
        'goose20263-isolate': 'fn main() { abort("rejected"); }',
        'haskell-ghc910-isolate': 'main :: IO ()\nmain = error "rejected"',
        'ruby40-isolate': 'raise "rejected"',
        'ruby-truffle40-isolate': 'raise "rejected"',
        **{key: 'assert False' for key in ('python314-isolate', 'pypy311-isolate', 'codon020-isolate')},
    }
    return families[name]  # Missing fixtures must never authorize publication.


def run(runtime, requested, fixtures, judge):
    c = '#include <stdio.h>\n#include <assert.h>\nint main(){for(int n=10;n<12;n++){printf("%d\\n",n);fflush(stdout);int x;assert(scanf("%d",&x)==1 && x==2*n);}}'
    python = 'for n in range(10,12):\n print(n, flush=True)\n assert int(input()) == n*2'
    rust = 'use std::io::{self,Write}; fn main(){for n in 10..12 {println!("{}",n);io::stdout().flush().unwrap();let mut s=String::new();io::stdin().read_line(&mut s).unwrap();assert_eq!(s.trim().parse::<i32>().unwrap(),2*n);}}'
    java = 'import java.util.*; public class Main {public static void main(String[] a){Scanner s=new Scanner(System.in);for(int n=10;n<12;n++){System.out.println(n);System.out.flush();assert s.nextInt()==2*n;}}}'
    csharp = 'using System; class Program { static void Main(){ for(int n=10;n<12;n++){ Console.WriteLine(n); Console.Out.Flush(); if(int.Parse(Console.ReadLine())!=2*n)throw new Exception(); } } }'
    go = 'package main\nimport "fmt"\nfunc main(){for n:=10;n<12;n++ {fmt.Println(n);var x int;if _,e:=fmt.Scan(&x);e!=nil||x!=2*n{panic("answer")}}}'
    nim = 'import std/strutils\nfor n in 10..11:\n echo n\n flushFile(stdout)\n doAssert parseInt(stdin.readLine())==2*n'
    haskell = 'import System.IO\nmain = mapM_ (\\n -> do { print n; hFlush stdout; x <- readLn; if x == 2*n then pure () else error "answer" }) ([10,11] :: [Int])'
    ruby = '$stdout.sync = true\n[10,11].each { |n| puts n; raise "answer" unless STDIN.gets.to_i == 2*n }'
    js = 'import {readSync,writeSync} from "node:fs"; const b=new Uint8Array(1); for(let n=10;n<12;n++){writeSync(1,n+"\\n");let s="";while(readSync(0,b,0,1,null)&&b[0]!==10)s+=String.fromCharCode(b[0]);if(Number(s)!==2*n)throw new Error("answer");}'
    deno = 'const b=new Uint8Array(1);for(let n=10;n<12;n++){Deno.stdout.writeSync(new TextEncoder().encode(n+"\\n"));let s="";while(Deno.stdin.readSync(b)&&b[0]!==10)s+=String.fromCharCode(b[0]);if(Number(s)!==2*n)throw new Error("answer");}'
    solution = '#include <cstdio>\nint main(){int n;while(scanf("%d",&n)==1){printf("%d\\n",2*n);fflush(stdout);}}'
    def check(name, source, code, want, *, submitted='cpp23-gcc-isolate', cases=None, memory=512):
        job = dict(runtime=submitted, runtimeDigest=runtime, source=source,
                   interactor=dict(runtime=name.removesuffix('-isolate'), source=code),
                   timeLimitMs=1000, memoryLimitMb=memory,
                   cases=cases or [dict(input='10', output='private expected')] * 2)
        result = judge(job, runtime)
        assert result['verdict'] == want, (name, want, result)
        print(name, 'interactive', want, 'OK', flush=True)
        return result
    allocate = solution.replace('int n;', 'volatile char* p=new char[MEMORY<<20];for(int i=0;i<(MEMORY<<20);i+=4096)p[i]=1;int n;')
    for memory in (64, 315, 512):
        check('cpp23-gcc-isolate', solution, c, 'AC', memory=memory)
        check('cpp23-gcc-isolate', allocate.replace('MEMORY', str(memory + 16)), c, 'MLE', memory=memory)
    for name in requested:
        # Text cannot answer an interactor, and it is excluded from judge code.
        if name == 'text-isolate':
            continue
        role_fixtures = {f['name']: f for f in fixtures[name] if f['name'] in ('interactive', 'response')}
        if 'interactive' in role_fixtures:
            code = role_fixtures['interactive']['source']
        elif name.startswith(('cpp', 'c23')):
            code = c
        elif name.startswith(('javascript-', 'typescript-')):
            code = deno if '-deno' in name else js
        else:
            code = {'rust2024-isolate': rust, 'java25-isolate': java,
                    'csharp14-isolate': csharp, 'go127-isolate': go, 'nim22-isolate': nim,
                    'haskell-ghc910-isolate': haskell, 'ruby40-isolate': ruby, 'ruby-truffle40-isolate': ruby,
                    'python314-isolate': python, 'pypy311-isolate': python, 'codon020-isolate': python}[name]
        check(name, solution, code, 'AC')
        check(name, solution.replace('2*n', '3*n'), code, 'WA')
        # Verify the added runtimes on the submitted side too, with two flushed exchanges.
        response = role_fixtures.get('response', {}).get('source')
        if name.startswith(('javascript-', 'typescript-')):
            if '-deno' in name:
                response = 'const b=new Uint8Array(1);for(let i=0;i<2;i++){let s="";while(Deno.stdin.readSync(b)&&b[0]!==10)s+=String.fromCharCode(b[0]);Deno.stdout.writeSync(new TextEncoder().encode(2*Number(s)+"\\n"));}'
            else:
                response = 'import {readSync,writeSync} from "node:fs";const b=new Uint8Array(1);for(let i=0;i<2;i++){let s="";while(readSync(0,b,0,1,null)&&b[0]!==10)s+=String.fromCharCode(b[0]);writeSync(1,2*Number(s)+"\\n");}'
        elif name.startswith('ruby'):
            response = '$stdout.sync = true\n2.times { puts STDIN.gets.to_i*2 }'
        elif name.startswith('haskell-'):
            response = 'import System.IO\nimport Control.Monad\nmain = replicateM_ 2 $ do { n <- readLn :: IO Int; print (2*n); hFlush stdout }'
        if response is not None:
            check('cpp23-gcc-isolate', response, c, 'AC', submitted=name)
        # Library fixtures also execute under the interactor's 256 MiB limit.
        for fixture in fixtures[name]:
            if fixture['verdict'] != 'AC':
                continue
            # Batch readers wait for EOF, but isolate holds the pipe open until
            # their peer exits. The flushed exchanges above cover interactive
            # stdin; do not reuse EOF-based batch programs as interactors.
            if fixture['name'] in ('stdin', 'commonjs'):
                continue
            source = '''#include <iostream>
#include <sstream>
#include <iterator>
#include <unistd.h>
#include <cassert>
int main(){std::cout<<INPUT<<std::flush;close(1);std::string data((std::istreambuf_iterator<char>(std::cin)),{});std::istringstream a(data),b(EXPECTED);std::string x,y;while(b>>y){assert(a>>x);assert(x==y);}assert(!(a>>x));}'''.replace('INPUT', json.dumps(fixture.get('input', ''))).replace('EXPECTED', json.dumps(fixture.get('output', '3')))
            check(name, source, fixture['source'], 'AC')
    private = '''import os,sys
assert len(sys.argv)==5
assert open(sys.argv[1]).read()=='10\\n'
assert open(sys.argv[2]).read()=='private expected\\n'
assert open(sys.argv[3]).read()
with open(sys.argv[4],'w') as f: f.write('1')
assert os.getuid()==60001
assert not os.path.exists('marker')
open('marker','w').close()
assert not os.path.exists('/run/judge/main')
assert not os.path.exists('/root/.aws/credentials')
print(10,flush=True)
assert int(input())==20
assert sys.stdin.read()==''
print('private diagnostic',file=sys.stderr)
'''
    private_solution = '''import os
assert os.getuid()==60000
for p in ('/box/test-input','/box/expected-output','/box/submission-source','/run/judge/main','/root/.aws/credentials'):
 assert not os.path.exists(p)
assert not os.path.exists('marker')
open('marker','w').close()
print(int(input())*2,flush=True)
'''
    result = check('python314-isolate', private_solution, private, 'AC', submitted='python314-isolate')
    assert 'private diagnostic' in result['checkerLog']
    # The 768 MiB allocation stays live on both sides, plus files and relay buffers.
    stress = '''import sys
memory=bytearray(MEMORY*1024*1024)
for offset in range(0,len(memory),4096): memory[offset]=1
with open('working-file','wb') as f: f.write(b'x'*(16*1024*1024))
'''
    check('python314-isolate', stress.replace('MEMORY', '420') + 'print(int(input())*2,flush=True)',
          stress.replace('MEMORY', '180') + 'print(10,flush=True)\nassert int(input())==20', 'AC', submitted='python314-isolate')
    for source, code, want in [
        ('int main(){}', 'invalid interactor', 'JE'),
        ('invalid solution', c, 'CE'),
        ('int main(){return 2;}', c, 'RE'),
        ('int main(){for(;;){}}', c, 'TLE'),
        (solution, 'int main(){for(;;){}}', 'JE'),
        (solution, '#include <unistd.h>\nint main(){sleep(60);}', 'TLE'),
        ('#include <unistd.h>\nint main(){sleep(60);}', 'int main(){}', 'TLE'),
        ('#include <cstdio>\nint main(){for(;;)putchar(65);}', '#include <cstdio>\nint main(){while(getchar()!=EOF){}}', 'OLE'),
        (solution, '#include <cstdio>\nint main(){for(;;)putchar(65);}', 'JE'),
        (solution, '#include <cstdio>\nint main(){for(;;)fputc(65,stderr);}', 'JE'),
    ]:
        check('cpp23-gcc-isolate', source, code, want, cases=[dict(input='', output='')])
    allocate = '#include <cstdlib>\nint main(){for(;;){volatile char* p=(char*)malloc(16<<20);if(p)for(int i=0;i<(16<<20);i+=4096)p[i]=1;}}'
    check('cpp23-gcc-isolate', allocate, c, 'MLE', cases=[dict(input='', output='')])
    check('cpp23-gcc-isolate', solution, allocate, 'JE', cases=[dict(input='', output='')])
    # Descendants cannot survive a completed case or prevent later submissions.
    fork = '#include <unistd.h>\n#include <cstdio>\nint main(){if(fork()==0){sleep(60);return 0;}puts("10");fflush(stdout);int x;return scanf("%d",&x)==1 && x==20 ? 0 : 1;}'
    check('cpp23-gcc-isolate', solution, fork, 'AC')
    check('python314-isolate', private_solution, private, 'AC', submitted='python314-isolate')
    run_twice(runtime, requested, fixtures, judge)


# Run-twice judge (legacy): argv is test-input expected-output submission-source score run handoff.
# Run 1 asks N, stores the submission's reply in the handoff; run 2 sends the handoff back and wants N.
# The decoy score 99 in run 1 shows that only run 2 sets the case score (the real score is 6).
TWICE_JUDGE = '''import sys
def reply():
    try:
        return int(sys.stdin.readline())
    except ValueError:
        sys.exit(1)
n = int(open(sys.argv[1]).read())
run, handoff = sys.argv[5], sys.argv[6]
with open(sys.argv[4], 'w') as f:
    f.write('99' if run == '1' else '6')
if run == '1':
    print(n, flush=True)
    x = reply()
    with open(handoff, 'w') as f:
        f.write(str(x))
    sys.exit(0 if x == n + 1000 else 1)
h = int(open(handoff).read())
print(h, flush=True)
sys.exit(0 if reply() == n else 1)
'''

# Tells the two runs apart by the value it receives: N in run 1, N + 1000 (the handoff) in run 2.
TWICE_SUBMISSION = '''x = int(input())
print(x + 1000 if x < 1000 else x - 1000, flush=True)
'''

# Records whether a file from another run is visible: 1 if not, 0 if so. The judge accepts only 1, in both runs.
TWICE_CARRY_SUBMISSION = '''import os
input()
seen = os.path.exists('carry')
open('carry', 'w').close()
print(0 if seen else 1, flush=True)
'''
TWICE_CARRY_JUDGE = '''import sys
print(0, flush=True)
try:
    answer = int(sys.stdin.readline())
except ValueError:
    sys.exit(1)
sys.exit(0 if answer == 1 else 1)
'''

# testlib: the test input is "0 N" and run 1 writes "1 <reply>" to tout, which is inf of run 2.
TWICE_TESTLIB_JUDGE = '''#include "testlib.h"
#include <iostream>
int main(int argc,char** argv){registerInteraction(argc,argv);
 int phase=inf.readInt();int n=inf.readInt();int expected=ans.readInt();
 std::cout<<n<<std::endl;
 int x=ouf.readInt();
 if(phase==0){
  tout<<1<<' '<<x<<std::endl;
  if(x!=n+1000)quitf(_wa,"first run");
  FIRST;}
 if(x!=expected)quitf(_wa,"second run");
 SECOND;}'''


def run_twice(runtime, passed, fixtures, judge):
    """Run-twice jobs on real boxes: handoff, skipped run 2, fresh box per run, run-2-only scores, both protocols."""
    if 'python314-isolate' not in passed:
        return
    from testlib_smoke import exact_aggregator

    def check(name, source, code, want, *, protocol=None, case=('7\n', '7\n'), scoring=None):
        judge_code = dict(runtime='python314' if protocol is None else 'cpp23-gcc', source=code)
        if protocol is not None:
            judge_code['protocol'] = protocol
        job = dict(runtime='python314-isolate', runtimeDigest=runtime, source=source, runTwice=judge_code,
                   timeLimitMs=1000, memoryLimitMb=512, cases=[dict(input=case[0], output=case[1])])
        if scoring is not None:
            group, points, aggregator = scoring
            job['cases'] = [dict(input=case[0], output=case[1], isSample=False, groups=[group])]
            job['scoring'] = dict(mode='aggregate', groups=[dict(name=group, points=points)],
                                  aggregator=dict(runtime='cpp23-gcc', source=aggregator))
        result = judge(job, runtime)
        assert result['verdict'] == want, (name, want, result)
        print(name, 'run-twice', want, 'OK', flush=True)
        return result

    skipped = '2回目: 実行しません'
    result = check('handoff', TWICE_SUBMISSION, TWICE_JUDGE, 'AC')
    assert skipped not in result.get('checkerLog', ''), result
    # A bad reply in run 1 is WA and run 2 does not start.
    result = check('run 1 wrong', 'input()\nprint("x", flush=True)\n', TWICE_JUDGE, 'WA')
    assert skipped in result.get('checkerLog', ''), result
    # A bad reply in run 2 alone is WA, after run 2 started.
    result = check('run 2 wrong', TWICE_SUBMISSION.replace('x - 1000', 'x - 999'), TWICE_JUDGE, 'WA')
    assert skipped not in result.get('checkerLog', ''), result
    # The second run gets a fresh box: the file written by run 1 is gone.
    check('fresh box', TWICE_CARRY_SUBMISSION, TWICE_CARRY_JUDGE, 'AC')
    if 'cpp23-gcc-isolate' not in passed:
        return
    # Aggregate mode: the case score comes from run 2 (6), not from run 1 (99).
    result = check('legacy score', TWICE_SUBMISSION, TWICE_JUDGE, 'AC',
                   scoring=('g', 10, exact_aggregator('1 1\n10\nAC 6 0 1 0\n', '6')))
    assert result['cases'][0].get('score') == 6 and result['scoring'] == dict(
        score=600, maxScore=1000, groups=[dict(score=600, maxScore=1000)]), result
    plain = TWICE_TESTLIB_JUDGE.replace('FIRST', 'quitf(_ok,"first")').replace('SECOND', 'quitf(_ok,"second")')
    check('testlib', TWICE_SUBMISSION, plain, 'AC', protocol='testlib', case=('0 7\n', '7\n'))
    points = TWICE_TESTLIB_JUDGE.replace('FIRST', 'quitp(11,"first")').replace('SECOND', 'quitp(37,"second")')
    result = check('testlib score', TWICE_SUBMISSION, points, 'AC', protocol='testlib', case=('0 7\n', '7\n'),
                   scoring=('all', 100, exact_aggregator('1 1\n100\nAC 37 0 1 0\n', '37')))
    assert result['cases'][0].get('score') == 37 and result['scoring'] == dict(
        score=3700, maxScore=10000, groups=[dict(score=3700, maxScore=10000)]), result
