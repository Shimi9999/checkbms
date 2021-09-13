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

	if len(flag.Args()) >= 3 {
		fmt.Println("Usage: checkbms [bmsPath/dirPath] [diffDirPath]")
		os.Exit(1)
	}

	var path string
	if len(flag.Args()) == 0 {
		path = "./"
	} else if len(flag.Args()) == 2 {
		if err := doDiffBmsDir(flag.Arg(0), flag.Arg(1)); err == nil {
			fmt.Println("Error: doDiffBmsDir:", err.Error())
			os.Exit(1)
		}
		os.Exit(0)
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
		bmsFile, err := checkbms.ReadBmsFile(path)
		if err != nil {
			fmt.Println("Error: ScanBms error:", err.Error())
			os.Exit(1)
		}
		err = bmsFile.ScanBmsFile()
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

func doDiffBmsDir(dirPath1, dirPath2 string) error {
	dirPath1, dirPath2 = filepath.Clean(dirPath1), filepath.Clean(dirPath2)
	_, err := os.Stat(dirPath1)
	if err != nil {
		return fmt.Errorf("Error: Path1 is wrong: %s", err.Error())
	}
	_, err = os.Stat(dirPath2)
	if err != nil {
		return fmt.Errorf("Error: Path2 is wrong: %s", err.Error())
	}

	err = checkbms.DiffBmsDirectories(dirPath1, dirPath2)
	if err != nil {
		return fmt.Errorf("Error: DiffBmsDirectories error: %s", err.Error())
	}
	return nil
}
