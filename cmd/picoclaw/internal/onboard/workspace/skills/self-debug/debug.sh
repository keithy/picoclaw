#!/bin/bash
case "$1" in
    logs)
        journalctl --user-unit picoclaw --no-pager -n ${2:-50}
        ;;
    status)
        systemctl --user status picoclaw
        ;;
    config)
        if command -v jq >/dev/null; then
            cat "$HOME/.picoclaw/config.json" | jq .
        else
            python3 -m json.tool "$HOME/.picoclaw/config.json"
        fi
        ;;
    *)
        echo "Usage: $0 {logs|status|config}"
        exit 1
        ;;
esac
