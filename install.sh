#!/bin/bash
# Link omarchy-agent-launcher into ~/.local/bin and migrate config from the
# old "agent-apps" name. Safe to re-run.
set -euo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
bin="$HOME/.local/bin"
mkdir -p "$bin"
ln -sfn "$here/bin/omarchy-agent-launcher" "$bin/omarchy-agent-launcher"
chmod +x "$here/bin/omarchy-agent-launcher"

old_list="$HOME/.config/omarchy/agent-apps.json"
new_list="$HOME/.config/omarchy/agent-launcher.json"
if [[ -f $old_list && ! -f $new_list ]]; then
  mv "$old_list" "$new_list"
  echo "Moved $old_list -> $new_list"
fi

# Old install leftovers
rm -f "$bin/agent-apps"
rm -rf "$HOME/.local/share/agent-apps"
[[ -f $HOME/.local/state/agent-apps/launch.log ]] && rm -rf "$HOME/.local/state/agent-apps"

echo "Installed: $bin/omarchy-agent-launcher -> $here/bin/omarchy-agent-launcher"
echo "Hyprland: o.rebind(\"SUPER + A\", ..., \"omarchy-agent-launcher default\")"
echo "          o.rebind(\"SUPER + SHIFT + A\", ..., \"omarchy-agent-launcher open\")"
