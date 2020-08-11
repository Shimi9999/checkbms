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

			var log string
			for _, bmsFile := range dir.BmsFiles {
	      if len(bmsFile.Logs) > 0 {
	        log += bmsFile.LogString()
	        log += fmt.Sprintf("\n\n")
	      }
	    }
	    if len(dir.Logs) > 0 {
	      log += dir.LogString()
	      log += fmt.Sprintf("\n\n")
	    }
			fmt.Printf("%s", log)
		}
	} else if checkbms.IsBmsPath(path) {
		bmsFile, err := checkbms.ScanBmsFile(path)
		if err != nil {
			fmt.Println("Error: loadBms error:", err.Error())
			os.Exit(1)
		}
		checkbms.CheckBmsFile(bmsFile)
		if len(bmsFile.Logs) > 0 {
			fmt.Println(bmsFile.LogString())
		}
	} else {
		fmt.Println("Error: Entered path is not bms file or directory")
		os.Exit(1)
	}
}
