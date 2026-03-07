---
name: self-config
description: Allows the agent to safely update its own configuration files
---

# Self-Config Skill Capabilities

1. presentation of the config.json with secrets redacted.
2. staging of a new config.json with utilities for patching, sorting, and diff and validation
3. safe service restarts with auto-rollback.

## Tools used
- `scripts/config.sh`: Presents redacted JSON files.
- `scripts/config-patch.sh.sh`: Tools for patching a staged config.new.json file.
- `scripts/config-safe-switch.sh.sh`: Manages service restarts with an auto-rollback timers.
- `jq`: Used internally for JSON manipulation.

## View Current Configuration with Secrets Redacted

```bash
./scripts/config.sh redacted
```

## View Current Configuration Summarised with Secrets Redacted

```bash
./scripts/config.sh summary
```

## Workflow: Safe Configuration Update

### 1. Start a Session
Initialize a staging environment. This redacts sensitive keys (tokens, passwords) into a separate hidden file so the agent doesn't see them in plain text during editing.
```bash
./scripts/config-patch.sh start
```

### 2. Apply Patches
Apply `jq` filters to the **staged** file. You can run this multiple times to iterate.
```bash
./scripts/config-patch.sh '.path.to.key = "new_value"'
```

### 3. Check Status
View the current state of the staging session.
```bash
./scripts/config-patch.sh status
```

### 4. View Config (Redacted)
Display the current staging file with secrets redacted.
```bash
./scripts/config-patch.sh config
```

### 5. Verify Changes
Review the diff between the original and the staged (redacted) version.
```bash
./scripts/config-patch.sh diff
```

### 6. View Summary
Show a filtered summary with only configured models, agents, enabled tools, devices, and heartbeat settings.
```bash
./scripts/config-patch.sh summary
```

### 7. Sort Keys (Optional)
Sort the keys in the staged file alphabetically.
```bash
./scripts/config-patch.sh sort
```

### 8. Switch & Test (The "Hot" Update)
Commit the patch, restart the service, and start a rollback timer.
```bash
./scripts/config-safe-switch.sh switch <service_name>
```
*Immediately* schedule an auto-rollback in case you lose connection:
```bash
nohup sh -c "sleep 60 && ./scripts/config-safe-switch.sh auto-rollback <service_name>" > /dev/null 2>&1 &
```

### 9. Confirm
If the agent is still working and the changes are correct, confirm the update to stop the rollback.
```bash
./scripts/config-safe-switch.sh confirm
```

### 10. Rollback (Manual)
If things fail and you need to revert immediately:
```bash
./scripts/config-safe-switch.sh rollback <service_name>
```

### 11. Reset (Abort before Switch)
If you make a mistake *before* switching, clear the staging files.
```bash
./scripts/config-patch.sh reset
```

### 12. Commit (Apply Changes)
Apply staged changes to the config file (used internally by `config-safe-switch switch`).
```bash
./scripts/config-patch.sh commit
```

## Safety Rules
1. **Always** use `start` before applying patches to sensitive files.
2. **Never** manually edit the `.secrets.json` file.
3. **Always** confirm your changes within the time limit after a `switch`.