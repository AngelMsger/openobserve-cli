package errors

// defaultGuidance returns the default hint and next-step commands for a
// category. Callers may override these via WithHint / WithNextSteps when more
// specific guidance is available.
func defaultGuidance(cat Category) (hint string, steps []string) {
	switch cat {
	case CategoryUsage:
		return "The command was invoked incorrectly. Check flags and arguments.",
			[]string{"openobserve-cli <command> --help"}
	case CategoryConfig:
		return "No usable configuration was found or it is invalid.",
			[]string{"openobserve-cli config init", "openobserve-cli config show"}
	case CategoryAuth:
		return "The server rejected the credentials. The password/token may be wrong.",
			[]string{"openobserve-cli auth status", "openobserve-cli config init"}
	case CategoryPermission:
		return "The credentials are valid but lack permission for this organization or resource.",
			[]string{"openobserve-cli org list", "Verify the account can access this org in the web UI."}
	case CategoryNotFound:
		return "The requested organization, stream or resource does not exist.",
			[]string{"openobserve-cli org list", "openobserve-cli stream list"}
	case CategoryConflict:
		return "The resource changed since it was last read (version conflict).",
			[]string{"Re-fetch the resource to get its current state, then retry."}
	case CategoryRateLimit:
		return "The server is rate limiting requests. Retry after a short wait.",
			[]string{"Wait and retry; narrow the time range or reduce --limit."}
	case CategoryNetwork:
		return "The server could not be reached (DNS, TLS or timeout).",
			[]string{"openobserve-cli doctor", "Check --base-url / OPENOBSERVE_URL and network connectivity."}
	case CategoryServer:
		return "The OpenObserve server returned an internal error.",
			[]string{"Retry later.", "openobserve-cli doctor"}
	case CategoryParse:
		return "A response could not be parsed or rendered.",
			[]string{"Retry with --format json and --verbose to inspect raw content."}
	default:
		return "An unexpected internal error occurred.",
			[]string{"Retry with --verbose for details."}
	}
}
