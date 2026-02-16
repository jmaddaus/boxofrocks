package cli

import "strings"

// reorderArgs moves flag arguments before positional arguments so that
// Go's flag package (which stops at the first non-flag) parses them all.
// It handles "-flag value", "--flag value", "-flag=value", and "--flag=value".
// Boolean flags (no separate value) are not used by create or update, so
// every flag is assumed to consume the next argument as its value.
func reorderArgs(args []string) []string {
	var flags, positional []string
	i := 0
	for i < len(args) {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			if strings.Contains(arg, "=") {
				// -flag=value or --flag=value
				flags = append(flags, arg)
				i++
			} else if i+1 < len(args) {
				// -flag value or --flag value
				flags = append(flags, arg, args[i+1])
				i += 2
			} else {
				// trailing flag with no value — pass through, flag.Parse will error
				flags = append(flags, arg)
				i++
			}
		} else {
			positional = append(positional, arg)
			i++
		}
	}
	return append(flags, positional...)
}
