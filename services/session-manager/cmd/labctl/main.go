// Command labctl validates Entropy lab specs against Docker, using the same
// harness as CI and the AI generator.
//
//	labctl validate [--image entropy-sandbox:latest] [--runtime runsc] spec.json...
//	labctl export LAB_ID        print a built-in lab as a spec to start from
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/entropy/services/session-manager/labs"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "validate":
		os.Exit(validate(os.Args[2:]))
	case "export":
		if len(os.Args) != 3 {
			usage()
		}
		l, ok := labs.Get(os.Args[2])
		if !ok {
			fmt.Fprintf(os.Stderr, "unknown lab %q\n", os.Args[2])
			os.Exit(1)
		}
		out, _ := json.MarshalIndent(labs.SpecFromLab(l, "# write the shortest correct fix here"), "", "  ")
		fmt.Println(string(out))
	default:
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: labctl validate [--image IMAGE] [--runtime RUNTIME] spec.json... | labctl export LAB_ID")
	os.Exit(2)
}

func validate(args []string) int {
	fs := flag.NewFlagSet("validate", flag.ExitOnError)
	image := fs.String("image", "entropy-sandbox:latest", "sandbox image")
	runtime := fs.String("runtime", "", "OCI runtime, for example runsc")
	_ = fs.Parse(args)
	if fs.NArg() == 0 {
		usage()
	}
	failed := 0
	for _, path := range fs.Args() {
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Printf("FAIL %s: %v\n", path, err)
			failed++
			continue
		}
		spec, err := labs.ParseSpec(data)
		if err != nil {
			fmt.Printf("FAIL %s: %v\n", path, err)
			failed++
			continue
		}
		if problems := spec.Validate(); len(problems) > 0 {
			fmt.Printf("FAIL %s (static):\n", path)
			for _, p := range problems {
				fmt.Printf("  - %s\n", p)
			}
			failed++
			continue
		}
		settle := time.Duration(spec.SettleSeconds) * time.Second
		if settle == 0 {
			settle = 3 * time.Second
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		report := labs.ValidateInSandbox(ctx, labs.NewDockerRunnerFactory(*image, *runtime), spec.Lab(), spec.ReferenceFix, settle)
		cancel()
		if !report.OK {
			fmt.Printf("FAIL %s (sandbox):\n", path)
			for _, p := range report.Problems {
				fmt.Printf("  - %s\n", p)
			}
			failed++
			continue
		}
		fmt.Printf("ok   %s: broken after setup, fixed by referenceFix\n", path)
	}
	return failed
}
