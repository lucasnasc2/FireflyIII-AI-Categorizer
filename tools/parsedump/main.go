// Command parsedump runs the merchant parser over a list of raw bank
// descriptions and prints exactly what the model would be shown.
//
// It is a development/diagnostic tool: it reads one description per line (from
// a file argument or stdin) and writes one TSV row per description, followed by
// a summary of rule coverage. Use it to check parser changes against a real
// export before rolling them out.
//
//	go run ./tools/parsedump -file descriptions.txt
//	python3 tools/audit.py descriptions | go run ./tools/parsedump
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/openaccountants/firefly-iii-ai-categorize/internal/classifier"
)

func main() {
	file := flag.String("file", "", "read descriptions from this file instead of stdin")
	quiet := flag.Bool("quiet", false, "only print the summary")
	flag.Parse()

	var in io.Reader = os.Stdin
	if *file != "" {
		f, err := os.Open(*file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "parsedump: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()
		in = f
	}

	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	counts := map[string]int{}
	total, noMerchant := 0, 0

	for scanner.Scan() {
		desc := strings.TrimSpace(scanner.Text())
		if desc == "" {
			continue
		}
		total++

		info := classifier.ParseDescription(desc)
		counts[methodLabel(info.Method)]++

		if info.Merchant == "" {
			noMerchant++
		}
		if !*quiet {
			fmt.Printf("%s\t%s\t%s\t%s\n", info.Merchant, methodLabel(info.Method), classifier.Redact(desc), desc)
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "parsedump: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "\ntotal=%d merchants parsed=%d unparsed=%d\n", total, total-noMerchant, noMerchant)

	labels := make([]string, 0, len(counts))
	for k := range counts {
		labels = append(labels, k)
	}
	sort.Slice(labels, func(i, j int) bool { return counts[labels[i]] > counts[labels[j]] })
	for _, k := range labels {
		fmt.Fprintf(os.Stderr, "  %-32s %4d  (%5.1f%%)\n", k, counts[k], 100*float64(counts[k])/float64(total))
	}
}

// methodLabel renders a payment method as a short stable label for counting.
func methodLabel(m classifier.PaymentMethod) string {
	if m == classifier.MethodUnknown {
		return "unmatched (whole description)"
	}
	return string(m)
}
