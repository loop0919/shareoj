# bash completion for shareoj. Load it from ~/.bashrc (bash 3.2 cannot source a process substitution):
#   eval "$(shareoj completions bash)"

_shareoj() {
	local cur=${COMP_WORDS[COMP_CWORD]} prev=${COMP_WORDS[COMP_CWORD - 1]} options=
	if ((COMP_CWORD == 1)); then
		if [[ $cur == -* ]]; then
			COMPREPLY=($(compgen -W '--help --version' -- "$cur"))
		else
			COMPREPLY=($(compgen -W 'guide init check pull push login logout version completions' -- "$cur"))
		fi
		return
	fi
	case $prev in
	--install-skill)
		COMPREPLY=($(compgen -W 'codex claude all' -- "$cur"))
		return
		;;
	--api | --expect-version | --site | --username)
		return
		;;
	esac
	case ${COMP_WORDS[1]} in
	guide) options='--install-skill' ;;
	pull | logout) options='--api' ;;
	push) options='--api --expect-version' ;;
	login) options='--api --username --google --no-browser --site' ;;
	version) return ;;
	esac
	if [[ $cur == -* ]]; then
		COMPREPLY=($(compgen -W "--help $options" -- "$cur"))
		return
	fi
	case ${COMP_WORDS[1]} in
	init | check | pull | push)
		# Adds a slash to directories; bash 3.2 on macOS has no compopt.
		compopt -o filenames 2>/dev/null
		COMPREPLY=($(compgen -d -- "$cur"))
		;;
	completions)
		COMPREPLY=($(compgen -W 'bash zsh fish' -- "$cur"))
		;;
	esac
}

complete -F _shareoj shareoj
