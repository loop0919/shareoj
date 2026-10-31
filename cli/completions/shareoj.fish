# fish completion for shareoj. Save it in fish's completion directory:
#   mkdir -p ~/.config/fish/completions && shareoj completions fish > ~/.config/fish/completions/shareoj.fish

set -l commands guide init check pull push login logout version completions
complete -c shareoj -f
complete -c shareoj -n 'not __fish_seen_subcommand_from version' -l help -d 'Show help'
complete -c shareoj -n "not __fish_seen_subcommand_from $commands" -l version -d 'Print the installed version'
complete -c shareoj -n "not __fish_seen_subcommand_from $commands" -a guide -d 'Print the authoring guide or install the authoring skill'
complete -c shareoj -n "not __fish_seen_subcommand_from $commands" -a init -d 'Create a problem directory from the template'
complete -c shareoj -n "not __fish_seen_subcommand_from $commands" -a check -d 'Check the configuration and files of a problem directory'
complete -c shareoj -n "not __fish_seen_subcommand_from $commands" -a pull -d 'Fetch a draft into a new directory'
complete -c shareoj -n "not __fish_seen_subcommand_from $commands" -a push -d 'Create or update a draft from a problem directory'
complete -c shareoj -n "not __fish_seen_subcommand_from $commands" -a login -d 'Sign in and save the tokens'
complete -c shareoj -n "not __fish_seen_subcommand_from $commands" -a logout -d 'Remove the saved credentials'
complete -c shareoj -n "not __fish_seen_subcommand_from $commands" -a version -d 'Print the installed version'
complete -c shareoj -n "not __fish_seen_subcommand_from $commands" -a completions -d 'Print a shell completion script'

complete -c shareoj -n '__fish_seen_subcommand_from guide' -l install-skill -x -a 'codex claude all' -d 'Install the authoring skill (user scope)'
complete -c shareoj -n '__fish_seen_subcommand_from init check pull push' -a '(__fish_complete_directories)'
complete -c shareoj -n '__fish_seen_subcommand_from pull push login logout' -l api -x -d 'ShareOJ API URL'
complete -c shareoj -n '__fish_seen_subcommand_from push' -l expect-version -x -d 'Save against a reviewed remote version after a conflict'
complete -c shareoj -n '__fish_seen_subcommand_from login' -l username -x -d 'Login email address'
complete -c shareoj -n '__fish_seen_subcommand_from login' -l google -d 'Sign in with Google in a browser'
complete -c shareoj -n '__fish_seen_subcommand_from login' -l no-browser -d 'Print the Google login URL without opening a browser'
complete -c shareoj -n '__fish_seen_subcommand_from login' -l site -x -d 'ShareOJ website origin for Google login'
complete -c shareoj -n '__fish_seen_subcommand_from completions' -a 'bash zsh fish'
