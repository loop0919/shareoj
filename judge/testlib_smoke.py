"""Real testlib programs, exercised with both existing C++23 compilers."""
import json


def exact_aggregator(expected, output):
    """An aggregator that compares its whole stdin byte for byte, so the input contract is exercised."""
    return ('#include <iostream>\n#include <iterator>\n#include <string>\n'
            'int main(){std::string s((std::istreambuf_iterator<char>(std::cin)),{});'
            'if(s!=' + json.dumps(expected) + ')return 1;std::cout<<' + json.dumps(output) + ';}')


def run(digest, runtimes, judge):
    for runtime in ('cpp23-gcc-isolate', 'cpp23-clang-isolate'):
        if runtime not in runtimes:
            continue
        def check(code, expected, *, interactive=False, answer=3, aggregate=None):
            solution = ('#include <cstdio>\nint main(){int n;if(scanf("%d",&n)!=1)return 1;printf("%d\\n",VALUE);fflush(stdout);}'
                        if interactive else '#include <cstdio>\nint main(){puts("VALUE");}').replace('VALUE', str(answer))
            job = dict(runtime=runtime, runtimeDigest=digest, source=solution,
                       timeLimitMs=1000, memoryLimitMb=512,
                       cases=[dict(input='3\n', output='3\n')] * 2)
            job['interactor' if interactive else 'checker'] = dict(runtime=runtime.removesuffix('-isolate'), protocol='testlib', source=code)
            if aggregate is not None:
                stdin, output, score = aggregate
                job['cases'] = [dict(input='3\n', output='3\n', isSample=False, groups=['all'])] * 2
                job['scoring'] = dict(mode='aggregate', groups=[dict(name='all', points=100)],
                                      aggregator=dict(runtime=runtime.removesuffix('-isolate'), source=exact_aggregator(stdin, output)))
            result = judge(job, digest)
            assert result['verdict'] == expected, (runtime, interactive, expected, result)
            if aggregate is not None and expected != 'JE':
                assert result['scoring'] == dict(score=score, maxScore=10000, groups=[dict(score=score, maxScore=10000)]), (runtime, result)
            print(runtime, 'testlib', 'interactor' if interactive else 'checker', expected, 'aggregate' if aggregate else '', 'OK', flush=True)
        checker = '''#include "testlib.h"
int main(int argc,char** argv){registerTestlibCmd(argc,argv);
 int n=inf.readInt();int expected=ans.readInt();int actual=ouf.readInt();
 if(n!=expected)quitf(_fail,"invalid test");
 if(actual!=expected)quitf(_wa,"different answer");
 quitf(_ok,"accepted");}'''
        interactor = '''#include "testlib.h"
#include <iostream>
int main(int argc,char** argv){registerInteraction(argc,argv);
 int n=inf.readInt();int expected=ans.readInt();std::cout<<n<<std::endl;
 int actual=ouf.readInt();tout<<actual<<std::endl;
 if(actual!=expected)quitf(_wa,"different answer");
 quitf(_ok,"accepted");}'''
        for interactive, code in ((False, checker), (True, interactor)):
            check(code, 'AC', interactive=interactive)
            check(code, 'WA', interactive=interactive, answer=4)
            for termination, expected in [('quitf(_pe,"format")', 'WA'), ('quitf(_fail,"broken judge")', 'JE'), ('quitp(0.5,"partial")', 'JE')]:
                check(code.replace('quitf(_ok,"accepted")', termination), expected, interactive=interactive)
            # Aggregate mode passes a result file and -appes, and reads integer points from it (ADR 0025).
            arguments = 'if(argc!=6 || std::string(argv[4])!="/box/result" || std::string(argv[5])!="-appes" || access("/box/score",F_OK)==0)return 3;\n'
            points = ('#include <string>\n#include <unistd.h>\n' + code.replace('{register', '{' + arguments + ' register')
                      .replace('quitf(_ok,"accepted")', 'quitp(37,"partial <&>")'))
            check(points, 'AC', interactive=interactive, aggregate=('2 1\n100\nAC 37 0 1 0\nAC 37 0 1 0\n', '74.509', 7450))
            check(code, 'WA', interactive=interactive, answer=4, aggregate=('2 1\n100\nWA - 0 1 0\nWA - 0 1 0\n', '0', 0))
            for termination in ('quitp(0.5,"partial")', 'quitf(_pc(1),"partially correct")'):
                check(code.replace('quitf(_ok,"accepted")', termination), 'JE', interactive=interactive, aggregate=('', '0', 0))
        # Check the actual argv/file contract, including binary contestant output.
        contract = '''#include "testlib.h"
#include <fstream>
#include <iterator>
#include <unistd.h>
int main(int argc,char** argv){
 if(argc!=4 || access("/box/submission-source",F_OK)==0 || access("/box/score",F_OK)==0)return 3;
 if(getchar()!=EOF)return 3;
 registerTestlibCmd(argc,argv);
 std::ifstream f(argv[2],std::ios::binary);
 std::string s((std::istreambuf_iterator<char>(f)),{});
 if(s!=std::string("3\\0\\r\\n",4))quitf(_fail,"output bytes changed");
 for(int i=0;i<4;i++)ouf.readChar();
 if(inf.readInt()!=3 || ans.readInt()!=3)quitf(_fail,"wrong file order");
 quitf(_ok,"file contract preserved");}'''
        result = judge(dict(runtime=runtime, runtimeDigest=digest,
            source='#include <cstdio>\nint main(){fwrite("3\\0\\r\\n",1,4,stdout);}',
            timeLimitMs=1000, memoryLimitMb=512, cases=[dict(input='3\n', output='3\n')],
            checker=dict(runtime=runtime.removesuffix('-isolate'), protocol='testlib', source=contract)), digest)
        assert result['verdict'] == 'AC', (runtime, 'testlib file contract', result)
        print(runtime, 'testlib binary file contract OK', flush=True)
