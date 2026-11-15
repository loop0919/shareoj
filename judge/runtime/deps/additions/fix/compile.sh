#!/bin/sh
# Build /box/main.fix as main.fix of a copy of the template, whose dependencies are fetched and type-checked already.
# Only main.fix comes from the submission: the project file, the lock and the dependencies are the operator's.
set -eu
R=/opt/judge-runtimes
P=/box/project
mkdir "$P"
# fix writes its caches into the project, and refuses a .fixlang that is a link. The dependencies' preliminary make
# keeps their lib.o only while it is newer than lib.c, so the copy keeps the modification times.
cp -R --preserve=timestamps "$R/fix-project/." "$P"
cp /box/main.fix "$P/main.fix"
cd "$P"
exec "$R/fix/fix" build -O max -o /box/main
