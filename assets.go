package herdrtriggers

import "embed"

//go:embed mods/triggers/main.js mods/triggers/mod.json mods/triggers/icon.svg mods/triggers/RECIPE.md skills/schedule-local/SKILL.md
var Assets embed.FS
