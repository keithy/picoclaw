#!/bin/bash

# config-safe-switch: A tool for safe service restarts with rollback capability.
# Usage:
#   config-safe-switch switch <service> [config.json]    - Commit patch, restart service, and create pending marker
#   config-safe-switch rollback <service> [config.json]  - Restore backup and restart service (unconditional)
#   config-safe-switch auto-rollback <service> [config.json] - Rollback ONLY if pending marker exists
#   config-safe-switch confirm [config.json]             - Mark changes as confirmed (remove pending marker)

# Determine script directory for relocatable operation
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SKILL_ROOT="$(dirname "$SCRIPT_DIR")"

# Source shared config and functions
source "$SCRIPT_DIR/config.sh"

COMMAND=$1
SERVICE_NAME=$2
CONFIG_FILE=$3

# Default config path: PICOCLAW_CONFIG env var, or ~/.picoclaw/config.json
if [ -z "$CONFIG_FILE" ]; then
    CONFIG_FILE="${PICOCLAW_CONFIG:-$HOME/.picoclaw/config.json}"
fi

if [ -z "$COMMAND" ]; then
    echo "Usage: config-safe-switch <switch|rollback|auto-rollback|confirm> [service] [config_file]"
    exit 1
fi

# Validate command
case "$COMMAND" in
    switch|rollback|auto-rollback|confirm)
        assert_is_identifier "$COMMAND" "Command must be a valid identifier" || exit 1
        ;;
    *)
        echo "Error: Unknown command: $COMMAND" >&2
        exit 1
        ;;
esac

# Validate service name if provided
if [ -n "$SERVICE_NAME" ]; then
    assert_is_identifier "$SERVICE_NAME" "Service name must be a valid identifier" || exit 1
fi

# Validate the config file path
assert_is_file_path "$CONFIG_FILE" "Config file path must be valid" || exit 1

# Determine the target filenames
if [[ "$CONFIG_FILE" == *.json ]]; then
    BASE="${CONFIG_FILE%.json}"
else
    BASE="${CONFIG_FILE}"
fi
PENDING_FILE="${BASE}.rollback_pending"

# Helper: Restart Service
restart_service() {
    if [ -n "$SERVICE_NAME" ]; then
        echo "Restarting service: $SERVICE_NAME"
        if systemctl --user is-active --quiet "$SERVICE_NAME" 2>/dev/null; then
            systemctl --user restart "$SERVICE_NAME"
        elif systemctl is-active --quiet "$SERVICE_NAME" 2>/dev/null; then
            systemctl restart "$SERVICE_NAME"
        else
            echo "Warning: Service $SERVICE_NAME is not active or not found (system or user)."
        fi
    fi
}

case "$COMMAND" in
    switch)
        # 1. Commit changes using config-patch.sh
        if "$SCRIPT_DIR/config-patch.sh" commit "$CONFIG_FILE"; then
            echo "Committed changes for $CONFIG_FILE"
        else
            echo "Error: Failed to commit changes. Aborting switch."
            exit 1
        fi

        # 2. Restart Service
        restart_service

        # 3. Create marker for the rollback timer
        touch "$PENDING_FILE"
        echo "Switch complete. Rollback pending file created: $PENDING_FILE"
        echo "To confirm: $SCRIPT_DIR/config-safe-switch.sh confirm $CONFIG_FILE"
        exit 0
        ;;

    rollback)
        # 1. Restore backup using config-patch.sh
        if "$SCRIPT_DIR/config-patch.sh" rollback "$CONFIG_FILE"; then
            echo "Rollback: Restored $CONFIG_FILE"
        else
            echo "Error: Failed to rollback $CONFIG_FILE."
            exit 1
        fi

        # 2. Restart Service
        restart_service

        # 3. Cleanup
        rm -f "$PENDING_FILE"
        exit 0
        ;;

    auto-rollback)
        if [ -f "$PENDING_FILE" ]; then
            echo "Timer expired and $PENDING_FILE still exists. Triggering rollback..."
            # Call the standard rollback logic
            "$0" rollback "$SERVICE_NAME" "$CONFIG_FILE"
        else
            echo "Rollback skipped: $PENDING_FILE not found (changes were confirmed)."
        fi
        exit 0
        ;;

    confirm)
        if [ -f "$PENDING_FILE" ]; then
            rm "$PENDING_FILE"
            echo "Confirmed: $CONFIG_FILE is stable. Rollback marker removed."
        else
            echo "Nothing to confirm: No pending rollback for $CONFIG_FILE."
        fi
        exit 0
        ;;

    *)
        echo "Unknown command: $COMMAND"
        exit 1
        ;;
esac
