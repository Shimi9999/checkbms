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

	if len(flag.Args()) == 2 {
		if err := doDiffBmsDir(flag.Arg(0), flag.Arg(1), *lang); err != nil {
			fmt.Println("Error: doDiffBmsDir:", err.Error())
			os.Exit(1)
		}
	} else {
		path := "./"
		if len(flag.Args()) == 1 {
			path = flag.Arg(0)
		}

		fInfo, err := os.Stat(path)
		if err != nil {
			fmt.Println("Error: Path is wrong:", err.Error())
			os.Exit(1)
		}
		path = filepath.Clean(path)

		if fInfo.IsDir() {
			if err := doCheckBmsDirectory(path, *doDiffCheck, *lang); err != nil {
				fmt.Println("Error: CheckBmsDirectory error:", err.Error())
				os.Exit(1)
			}
		} else if checkbms.IsBmsFile(path) {
			if err := doCheckBmsFile(path, *lang); err != nil {
				fmt.Println("Error: CheckBmsFile error:", err.Error())
				os.Exit(1)
			}
		} else {
			fmt.Println("Error: Entered path is not bms file or directory")
			os.Exit(1)
		}
	}
}

func doCheckBmsDirectory(path string, doDiffCheck bool, lang string) error {
	bmsDirs, err := checkbms.ScanDirectory(path)
	if err != nil {
		return fmt.Errorf("Error: scanDirectory error: %s", err.Error())
	}
	for _, dir := range bmsDirs {
		checkbms.CheckBmsDirectory(&dir, doDiffCheck)

		var log string
		for _, bmsFile := range dir.BmsFiles {
			if len(bmsFile.Logs) > 0 {
				log += bmsFile.LogStringWithLang(false, lang)
				log += "\n\n"
			}
		}
		for _, bmsonFile := range dir.BmsonFiles {
			if len(bmsonFile.Logs) > 0 {
				log += bmsonFile.LogStringWithLang(false, lang)
				log += "\n\n"
			}
		}
		if len(dir.Logs) > 0 {
			log += dir.LogStringWithLang(false, lang)
			log += "\n\n"
		}
		fmt.Printf("%s", log)
	}
	return nil
}

func doCheckBmsFile(path, lang string) error {
	bmsFileBase, err := checkbms.ReadBmsFileBase(path)
	if err != nil {
		return fmt.Errorf("Error: ReadBmsFile error: %s", err.Error())
	}
	if checkbms.IsBmsonFile(path) {
		bmsonFile := checkbms.NewBmsonFile(bmsFileBase)
		err = bmsonFile.ScanBmsonFile()
		if err != nil {
			return fmt.Errorf("Error: ScanBmsonFile error: %s", err.Error())
		}
		checkbms.CheckBmsonFile(bmsonFile)
		if len(bmsonFile.Logs) > 0 {
			fmt.Println(bmsonFile.LogStringWithLang(false, lang))
		}
	} else {
		bmsFile := checkbms.NewBmsFile(bmsFileBase)
		err = bmsFile.ScanBmsFile()
		if err != nil {
			return fmt.Errorf("Error: ScanBmsFile error: %s", err.Error())
		}
		checkbms.CheckBmsFile(bmsFile)
		if len(bmsFile.Logs) > 0 {
			fmt.Println(bmsFile.LogStringWithLang(false, lang))
		}
	}
	return nil
}

func doDiffBmsDir(dirPath1, dirPath2, lang string) error {
	dirPath1, dirPath2 = filepath.Clean(dirPath1), filepath.Clean(dirPath2)
	_, err := os.Stat(dirPath1)
	if err != nil {
		return fmt.Errorf("Error: Path1 is wrong: %s", err.Error())
	}
	_, err = os.Stat(dirPath2)
	if err != nil {
		return fmt.Errorf("Error: Path2 is wrong: %s", err.Error())
	}

	result, err := checkbms.DiffBmsDirectories(dirPath1, dirPath2)
	if err != nil {
		return fmt.Errorf("Error: DiffBmsDirectories error: %s", err.Error())
	}
	fmt.Println(result.LogStringWithLang(lang))
	return nil
}
