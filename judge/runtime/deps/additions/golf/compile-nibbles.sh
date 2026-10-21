#!/bin/sh
set -eu
cd /box
/opt/judge-runtimes/golf/nibbles -hs main.nbl
exec /opt/judge-runtimes/ghc/bin/ghc -O -j1 -outputdir /box/nibbles-build -i/opt/judge-runtimes/golf/nibbles-lib @/opt/judge-runtimes/golf/nibbles.args /box/out.hs -o /box/main
