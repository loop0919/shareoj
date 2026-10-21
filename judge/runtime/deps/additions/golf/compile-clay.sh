#!/bin/sh
set -eu
/opt/judge-runtimes/golf/clay < /box/main.clay > /box/main.cpp
exec /opt/judge-runtimes/gcc/bin/g++ -std=c++23 -O2 -pipe -I/opt/judge-runtimes/include -Wl,-rpath,/opt/judge-runtimes/gcc/lib64 /box/main.cpp -o /box/main
