package flagx

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
)

var (
	envPrefix       = flag.String("env.prefix", "", "Name prefix of environment variables that interact with flags.")
	options         = map[string]*option{}
	funcsAfterParse []*funcAfterParse
)

func newFlag(name string, opts []Option) *option {
	x := (&option{name: name}).apply(opts)
	options[name] = x
	return x
}

// NewBool creates a new bool flag.
func NewBool(name string, value bool, usage string, opts ...Option) *bool {
	x := newFlag(name, opts)
	b := flag.Bool(name, value, x.usage(name, usage))
	return b
}

// NewString creates a new string flag.
func NewString(name string, value string, usage string, opts ...Option) *string {
	x := newFlag(name, opts)
	s := flag.String(name, value, x.usage(name, usage))
	return s
}

// NewInt creates a new int flag.
func NewInt(name string, value int, usage string, opts ...Option) *int {
	x := newFlag(name, opts)
	i := flag.Int(name, value, x.usage(name, usage))
	return i
}

// NewInt64 creates a new int64 flag.
func NewInt64(name string, value int64, usage string, opts ...Option) *int64 {
	x := newFlag(name, opts)
	i64 := flag.Int64(name, value, x.usage(name, usage))
	return i64
}

// NewFloat creates a new float64 flag.
func NewFloat(name string, value float64, usage string, opts ...Option) *float64 {
	x := newFlag(name, opts)
	f := flag.Float64(name, value, x.usage(name, usage))
	return f
}

// WriteFlags writes all the explicitly set flags to w.
func WriteFlags(w io.Writer) {
	Visit(func(name, value string) {
		fmt.Fprintf(w, "-%s=%q\n", name, value)
	})
}

// Visit the flags name and values set in command line
func Visit(fn func(string, string)) {
	flag.Visit(func(f *flag.Flag) {
		lname := strings.ToLower(f.Name)
		value := f.Value.String()
		if IsSecretFlag(lname) {
			value = toSecret(value)
		}
		fn(lname, value)
	})
}

// Visit all the flag name and values, including those not set in command line.
func VisitAll(fn func(string, string)) {
	flag.VisitAll(func(f *flag.Flag) {
		lname := strings.ToLower(f.Name)
		value := f.Value.String()
		if IsSecretFlag(lname) {
			value = toSecret(value)
		}
		fn(lname, value)
	})
}

// Parse parses environment vars(if env.prefix provided) and command-line flags.
//
// Flags set via command-line override flags set via environment vars.
//
// This function must be called instead of flag.Parse() before using any flags in the program.
func Parse() {
	ParseFlagSet(flag.CommandLine, os.Args[1:])
}

// ParseEnv parses environment vars(if env.prefix provided) and command-line flags.
func ParseEnv() {
	fs := flag.CommandLine

	// Remember explicitly set command-line flags.
	flagsSet := make(map[string]bool)
	fs.Visit(func(f *flag.Flag) {
		flagsSet[f.Name] = true
	})

	// Obtain the remaining flag values from environment vars.
	fs.VisitAll(func(f *flag.Flag) {
		if flagsSet[f.Name] {
			// The flag is explicitly set via command-line.
			return
		}
		// Get flag value from environment var.
		ok, err := setFlagFromEnv(f)
		if err != nil {
			fmt.Fprintf(os.Stderr, err.Error())
			os.Exit(1)
		}
		if ok {
			flagsSet[f.Name] = true
		}
	})

	for _, fap := range funcsAfterParse {
		fap.fn(flagsSet[fap.flagName], func(value string) error {
			if value == "" {
				return nil
			}
			flagsSet[fap.flagName] = true
			return fs.Set(fap.flagName, value)
		})
	}

	// Check if any required flag is not set.
	fs.VisitAll(func(f *flag.Flag) {
		if flagsSet[f.Name] {
			// The flag is explicitly set via command-line or environment or followed flag.
			return
		}

		if fx, ok := options[f.Name]; ok && fx.required {
			fmt.Fprintf(os.Stderr, "argument %q is required, run command with --%s or set via %s environment variable\n", f.Name, f.Name, FlagEnvName(f.Name))
			os.Exit(1)
		}
	})
}

// ParseFlagSet parses the given args into the given fs.
func ParseFlagSet(fs *flag.FlagSet, args []string) {
	fs.Usage = func() { Usage(fmt.Sprintf("Usage of %s:", os.Args[0])) }
	if err := fs.Parse(args); err != nil {
		log.Fatalf("cannot parse flags %q: %s", args, err)
	}

	// Remember explicitly set command-line flags.
	flagsSet := make(map[string]bool)
	fs.Visit(func(f *flag.Flag) {
		flagsSet[f.Name] = true
	})

	// Obtain the remaining flag values from environment vars.
	fs.VisitAll(func(f *flag.Flag) {
		if flagsSet[f.Name] {
			// The flag is explicitly set via command-line.
			return
		}
		// Get flag value from environment var.
		ok, err := setFlagFromEnv(f)
		if err != nil {
			fmt.Fprintf(os.Stderr, err.Error())
			os.Exit(1)
		}
		if ok {
			flagsSet[f.Name] = true
		}
	})

	for _, fap := range funcsAfterParse {
		fap.fn(flagsSet[fap.flagName], func(value string) error {
			if value == "" {
				return nil
			}
			flagsSet[fap.flagName] = true
			return fs.Set(fap.flagName, value)
		})
	}

	// Check if any required flag is not set.
	fs.VisitAll(func(f *flag.Flag) {
		if flagsSet[f.Name] {
			// The flag is explicitly set via command-line or environment or followed flag.
			return
		}

		if fx, ok := options[f.Name]; ok && fx.required {
			fmt.Fprintf(os.Stderr, "argument %q is required, run command with --%s or set via %s environment variable\n", f.Name, f.Name, FlagEnvName(f.Name))
			os.Exit(1)
		}
	})
}

func setFlagFromEnv(f *flag.Flag) (bool, error) {
	fx, ok := options[f.Name]
	if !ok {
		return false, nil
	}

	envNames := append([]string{FlagEnvName(f.Name)}, fx.envs...)

	for _, envName := range envNames {
		if v := os.Getenv(envName); v != "" {
			if err := f.Value.Set(v); err != nil {
				return false, fmt.Errorf("cannot set flag %s to %q, which is read from env var %q: %s", f.Name, v, envName, err)
			}
			return true, nil
		}
	}

	return false, nil
}

func FlagEnvName(name string) string {
	return strings.ToUpper(*envPrefix + strings.ReplaceAll(name, ".", "_"))
}

// Option for flags
type Option func(*option)

type option struct {
	name     string
	required bool
	envs     []string
	isSet    bool
}

func (f *option) apply(opts []Option) *option {
	for _, opt := range opts {
		opt(f)
	}
	return f
}

func (f *option) usage(name string, description string) string {
	usage := description
	if len(f.envs) > 0 {
		usage += fmt.Sprintf(" (env: %s)", strings.Join(f.envs, ", "))
	} else {
		usage += fmt.Sprintf(" (env: %s)", FlagEnvName(name))
	}
	if f.required {
		usage += " (required)"
	}
	return usage
}

// Env customize environment for flag.
func Env(env string) Option {
	return func(f *option) { f.envs = append(f.envs, env) }
}

// Required mark the flag MUST BE set via command line or environment
func Required() Option {
	return func(f *option) { f.required = true }
}

// AfterParse set a function to be called after parsing flags.
// The function must not block.
func AfterParse(f func(isSet bool, set func(string) error)) Option {
	return func(o *option) { funcsAfterParse = append(funcsAfterParse, &funcAfterParse{flagName: o.name, fn: f}) }
}

type funcAfterParse struct {
	flagName string
	fn       func(isSet bool, set func(string) error)
}
