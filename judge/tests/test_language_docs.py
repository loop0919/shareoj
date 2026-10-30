import re
import shlex
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / 'runner'))
from runtimes import RUNTIMES

DOC = Path(__file__).resolve().parents[2] / 'web/app/content/docs/languages.md'


def command_text(argv):
    # Shell quoting that keeps the Python one-liners readable.
    return ' '.join(arg if shlex.quote(arg) == arg else f'"{arg}"' if "'" in arg else f"'{arg}'" for arg in argv)


def section(text, heading):
    return text.split(heading)[1].split('\n## ')[0]


def documented_ids(text):
    ids = {}
    for line in section(text, '## 言語とバージョン').splitlines():
        cells = [cell.strip() for cell in line.strip().strip('|').split(' | ')]
        if len(cells) == 3 and cells[2].startswith('`'):
            ids[cells[0]] = cells[2].strip('`')
    return ids


def command_lists(text):
    # Language name -> {label: command} for the reader-facing list and the folded internal list.
    body = section(text, '## コンパイルと実行')
    reader, internal = body.split(':::details')
    lists = []
    for part in (reader, internal):
        commands, name = {}, None
        for line in part.splitlines():
            if line.startswith('- '):
                name = line[2:]
                commands[name] = {}
            elif match := re.fullmatch(r'  - (.+?): `(.+)`', line):
                commands[name][match[1]] = match[2]
        lists.append(commands)
    return lists


class LanguageDocsTests(unittest.TestCase):
    def test_language_table_and_commands_cover_every_runtime(self):
        text = DOC.read_text()
        ids = documented_ids(text)
        reader, internal = command_lists(text)
        self.assertEqual(sorted(ids.values()), sorted(key.removesuffix('-isolate') for key in RUNTIMES))
        self.assertEqual(list(reader), list(ids))
        self.assertEqual(list(internal), list(ids))
        for name, commands in internal.items():
            with self.subTest(name):
                self.assertEqual(commands.keys(), reader[name].keys())
                self.assertEqual(list(commands)[-1], '実行')

    def test_internal_commands_are_the_exact_runtime_commands(self):
        text = DOC.read_text()
        ids = documented_ids(text)
        _, internal = command_lists(text)
        for name, commands in internal.items():
            runtime = RUNTIMES[ids[name] + '-isolate']
            run = commands.pop('実行')
            # Runtimes without a compile step list only the run command.
            compile_ = next(iter(commands.values()), command_text(['/usr/bin/true']))
            for documented, argv in ((compile_, runtime['compile']), (run, runtime['run'])):
                with self.subTest(name, argv=argv[0]):
                    self.assertEqual(shlex.split(command_text(argv)), argv)
                    self.assertEqual(documented, command_text(argv))


if __name__ == '__main__':
    unittest.main()
