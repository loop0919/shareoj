#!/bin/sh
# Compile /box/main.gleam as the module main of the package judge; its dependencies are compiled already.
set -eu
R=/opt/judge-runtimes
P=/box/project
export PATH="$R/erlang/bin:/usr/bin:/bin"
# gleam compiles the generated Erlang with escript; that VM gets the run's limits too. The JIT's default double
# mapping is a memfd larger than the compile's file size limit.
export ERL_FLAGS='+S 1:1 +SDcpu 1:1 +SDio 1 +A 1 +sbwt none +sbwtdcpu none +sbwtdio none +JMsingle true'
mkdir -p "$P/src"
cp "$R/gleam-project/gleam.toml" "$P/gleam.toml"
cp /box/main.gleam "$P/src/main.gleam"
# Not gleam build: it locks the build directory with flock, which isolate refuses (ENOSYS). compile-package
# takes no lock and reads the template's compiled dependencies in place.
"$R/gleam/gleam" compile-package --target erlang --package "$P" --out "$P/out" --lib "$R/gleam-project/build/dev/erlang"
# Without pub fn main() the program would only fail when it runs.
"$R/erlang/bin/erl" -mode minimal -boot no_dot_erlang -noshell -eval '
    {ok, {main, [{exports, Exports}]}} = beam_lib:chunks("/box/project/out/ebin/main.beam", [exports]),
    lists:member({main, 0}, Exports) orelse begin
        io:put_chars(standard_error, "error: main.gleam must define pub fn main()\n"),
        halt(1)
    end,
    halt(0).'
cp "$P/out/ebin/main.beam" /box/main.beam
