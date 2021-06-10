package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Shimi9999/checkbms"
)

func main() {
	doDiffCheck := flag.Bool("diff", false, "check difference flag")
	lang := flag.String("lang", "en", "log language")
	flag.Parse()

	if len(flag.Args()) >= 2 {
		fmt.Println("Usage: checkbms [bmspath/dirpath]")
		os.Exit(1)
	}

	var path string
	if len(flag.Args()) == 0 {
		path = "./"
	} else {
		path = flag.Arg(0)
	}
	fInfo, err := os.Stat(path)
	if err != nil {
		fmt.Println("Error: Path is wrong:", err.Error())
		os.Exit(1)
	}
	path = filepath.Clean(path)

	if fInfo.IsDir() {
		bmsDirs, err := checkbms.ScanDirectory(path)
		if err != nil {
			fmt.Println("Error: scanDirectory error:", err.Error())
			os.Exit(1)
		}
		for _, dir := range bmsDirs {
			checkbms.CheckBmsDirectory(&dir, *doDiffCheck)

			var log string
			for _, bmsFile := range dir.BmsFiles {
				if len(bmsFile.Logs) > 0 {
					log += bmsFile.LogStringWithLang(false, *lang)
					log += "\n\n"
				}
			}
			if len(dir.Logs) > 0 {
				log += dir.LogStringWithLang(false, *lang)
				log += "\n\n"
			}
			fmt.Printf("%s", log)
		}
	} else if checkbms.IsBmsFile(path) {
		bmsFile, err := checkbms.ScanBmsFile(path)
		if err != nil {
			fmt.Println("Error: ScanBms error:", err.Error())
			os.Exit(1)
		}
		checkbms.CheckBmsFile(bmsFile)
		if len(bmsFile.Logs) > 0 {
			fmt.Println(bmsFile.LogStringWithLang(false, *lang))
		}
	} else {
		fmt.Println("Error: Entered path is not bms file or directory")
		os.Exit(1)
	}
}
