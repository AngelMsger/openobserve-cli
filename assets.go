// Package openobservecli is the module root. It exists only to embed packaged
// assets — the companion `openobserve` Skill — into the CLI binary, so that
// `openobserve-cli skill install` can deploy a version-matched copy regardless
// of how the binary itself was installed (npm, go install, prebuilt, source).
package openobservecli

import "embed"

// SkillFS holds the companion Skill, rooted at "skills/openobserve".
//
//go:embed all:skills/openobserve
var SkillFS embed.FS

// SkillRoot is the path within SkillFS at which the Skill is rooted.
const SkillRoot = "skills/openobserve"
