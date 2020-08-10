package main

import (
	"os"
	"fmt"
	"flag"
	"path/filepath"

	"github.com/Shimi9999/checkbms"
)

func main() {
	doDiffCheck := flag.Bool("diff", false, "check difference flag")
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

			for _, bmsFile := range dir.BmsFiles {
				if len(bmsFile.Logs) > 0 {
					fmt.Printf("# BmsFile checklog: %s\n", bmsFile.Path)
					bmsFile.Logs.Print()
					fmt.Println("")
				}
			}
			if len(dir.Logs) > 0 {
				dirPath := filepath.Clean(dir.Path)
				if dirPath == "." {
					dirPath, _ = filepath.Abs(dirPath)
					dirPath = filepath.Base(dirPath)
				}
				fmt.Printf("## BmsDirectory checklog: %s\n", dirPath)
				dir.Logs.Print()
				fmt.Println("")
			}
		}
	} else if checkbms.IsBmsPath(path) {
		bmsFile, err := checkbms.ScanBmsFile(path)
		if err != nil {
			fmt.Println("Error: loadBms error:", err.Error())
			os.Exit(1)
		}
		checkbms.CheckBmsFile(bmsFile)
		if len(bmsFile.Logs) > 0 {
			fmt.Printf("# BmsFile checklog: %s\n", bmsFile.Path)
			bmsFile.Logs.Print()
			fmt.Println("")
		}
	} else {
		fmt.Println("Error: Entered path is not bms file or directory")
		os.Exit(1)
	}
}
