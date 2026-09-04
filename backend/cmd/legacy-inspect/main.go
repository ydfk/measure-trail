package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/ydfk/measure-trail/backend/internal/legacy"
)

func main() {
	source := flag.String("source", "", "旧 slimtrack SQLite 文件路径")
	reportPath := flag.String("report", "", "dry-run 报告 JSON 输出路径；为空则写到标准输出")
	flag.Parse()
	if *source == "" {
		fmt.Fprintln(os.Stderr, "必须提供 --source")
		os.Exit(2)
	}
	report, err := legacy.Inspect(*source)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	contents, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		panic(err)
	}
	contents = append(contents, '\n')
	if *reportPath == "" {
		os.Stdout.Write(contents)
		return
	}
	if err := os.WriteFile(*reportPath, contents, 0o600); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
