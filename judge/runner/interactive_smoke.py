"""Run under the production service limits, with the worker stopped."""
import json
from pathlib import Path


def failure_program(name):
    if name.startswith(('cpp', 'c23')):
        return '#include <assert.h>\nint main(){assert(0);}'
    if name.startswith(('javascript-', 'typescript-')):
        return 'throw new Error("rejected");'
    families = {
        'rust2024-isolate': 'fn main(){assert!(false);}',
        'java24-isolate': 'public class Main { public static void main(String[] args){assert false;} }',
        'java25-isolate': 'public class Main { public static void main(String[] args){assert false;} }',
        'csharp14-isolate': 'class Program { static void Main(){throw new System.Exception();} }',
        'go127-isolate': 'package main\nfunc main(){panic("rejected")}',
        'nim22-isolate': 'doAssert false',
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
    def check(name, source, code, want, *, submitted='cpp17-isolate', cases=None, memory=512):
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
        check('cpp17-isolate', solution, c, 'AC', memory=memory)
        check('cpp17-isolate', allocate.replace('MEMORY', str(memory + 16)), c, 'MLE', memory=memory)
    for name in requested:
        if name.startswith(('cpp', 'c23')):
            code = c
        elif name.startswith(('javascript-', 'typescript-')):
            code = deno if '-deno' in name else js
        else:
            code = {'rust2024-isolate': rust, 'java24-isolate': java, 'java25-isolate': java,
                    'csharp14-isolate': csharp, 'go127-isolate': go, 'nim22-isolate': nim,
                    'haskell-ghc910-isolate': haskell, 'ruby40-isolate': ruby, 'ruby-truffle40-isolate': ruby,
                    'python314-isolate': python, 'pypy311-isolate': python, 'codon020-isolate': python}[name]
        check(name, solution, code, 'AC')
        check(name, solution.replace('2*n', '3*n'), code, 'WA')
        # Verify the added runtimes on the submitted side too, with two flushed exchanges.
        response = None
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
            check('cpp17-isolate', response, c, 'AC', submitted=name)
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
assert open(sys.argv[1]).read()=='10'
assert open(sys.argv[2]).read()=='private expected'
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
        check('cpp17-isolate', source, code, want, cases=[dict(input='', output='')])
    allocate = '#include <cstdlib>\nint main(){for(;;){volatile char* p=(char*)malloc(16<<20);if(p)for(int i=0;i<(16<<20);i+=4096)p[i]=1;}}'
    check('cpp17-isolate', allocate, c, 'MLE', cases=[dict(input='', output='')])
    check('cpp17-isolate', solution, allocate, 'JE', cases=[dict(input='', output='')])
    # Descendants cannot survive a completed case or prevent later submissions.
    fork = '#include <unistd.h>\n#include <cstdio>\nint main(){if(fork()==0){sleep(60);return 0;}puts("10");fflush(stdout);int x;return scanf("%d",&x)==1 && x==20 ? 0 : 1;}'
    check('cpp17-isolate', solution, fork, 'AC')
    check('python314-isolate', private_solution, private, 'AC', submitted='python314-isolate')
