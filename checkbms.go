package main

import (
	"os"
	"fmt"
	"flag"
	"strings"
	"strconv"
	"bufio"
	"bytes"
	"regexp"
	"math"
	"math/big"
	"io/ioutil"
	"path/filepath"
	"unicode/utf8"

	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/transform"
	"github.com/saintfish/chardet"
	/*"./bmsobject"
	  "./bmsloader"*/
)

type File struct {
	Path string
}

type Directory struct {
	File
	BmsFiles []BmsFile
	/*SoundFiles []NonBmsFile
	ImageFiles []NonBmsFile  // TODO
	OtherFiles []NonBmsFile*/
	NonBmsFiles []NonBmsFile
	Directories []Directory
}

func NewDirectory(path string) *Directory {
	var d Directory
	d.Path = path
	d.BmsFiles = make([]BmsFile, 0)
	d.NonBmsFiles = make([]NonBmsFile, 0)
	d.Directories = make([]Directory, 0)
	return &d
}

type definition struct {
	Command string
	Value string
}
type BmsFile struct {
	File
	Header map[string]string
	HeaderWav []definition
	HeaderBmp []definition
	HeaderNumbering []definition
	Pattern []definition
	Keymode int // 5, 7, 9, 10, 14, 24, 48
	TotalNotes int
	Logs []string
}
func NewBmsFile(path string) *BmsFile {
	var bf BmsFile
	bf.Path = path
	bf.Header = make(map[string]string, 0)
	bf.HeaderWav = make([]definition, 0)
	bf.HeaderBmp = make([]definition, 0)
	bf.HeaderNumbering = make([]definition, 0)
	bf.Pattern = make([]definition, 0)
	bf.Logs = make([]string, 0)
	return &bf
}
func (bf BmsFile) CalculateDefaultTotal() float64 {
	tn := float64(bf.TotalNotes)
	if bf.Keymode >= 24 {
		return math.Max(300.0, 7.605 * (tn + 100.0) / (0.01 * tn + 6.5))
	} else {
		return math.Max(260.0, 7.605 * tn / (0.01 * tn + 6.5))
	}
}

type NonBmsFile struct {
	File
	Used bool
}
func NewNonBmsFile(path string) *NonBmsFile {
	var nbf NonBmsFile
	nbf.Path = path
	return &nbf
}

type CommandType int
const (
	Int = iota
	Float
	String
	Path
)
type CommandNecessity int
const (
	Necessary = iota
	Semi_necessary
	Unnecessary
)
type Command struct {
	Name string
	Type CommandType
	Necessity CommandNecessity
	BoundaryValue interface{} // Intなら0と100,-10とnilとか。Stringなら*とか(多分不要)。Pathなら許容する拡張子.oog、.wav
	//Check func(string) *[]string // 引数の値に対してチェックをしてエラーメッセージ(string)のスライスを返す関数
}
func (c Command) IsInRange(value string) (bool, error) {
	switch c.Type {
	case Int:
		intValue, err := strconv.Atoi(value)
		if err != nil {
			return false, err
		}
		if bv, ok := c.BoundaryValue.(*[]int); ok && len(*bv) >= 2 {
			if intValue >= (*bv)[0] && intValue <= (*bv)[1] {
				return true, nil
			} else {
				return false, nil
			}
		} else {
			return false, fmt.Errorf("Error IsInRange: BoundaryValue is invalid")
		}
	case Float:
		floatValue, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return false, err
		}
		if bv, ok := c.BoundaryValue.(*[]float64); ok && len(*bv) >= 2 {
			if floatValue >= (*bv)[0] && floatValue <= (*bv)[1] {
				return true, nil
			} else {
				return false, nil
			}
		} else {
			return false, fmt.Errorf("Error IsInRange: BoundaryValue is invalid")
		}
	case String:
		if c.BoundaryValue == nil {
			return true, nil
		}
		if bv, ok := c.BoundaryValue.(*[]string); ok && len(*bv) >= 1 {
			if regexp.MustCompile((*bv)[0]).MatchString(value) { // 条件複数個にする？単数にする？
				return true, nil
			}
		} else {
			return false, fmt.Errorf("Error IsInRange: BoundaryValue is invalid")
		}
		return false, nil
	case Path:
		if bv, ok := c.BoundaryValue.(*[]string); ok && len(*bv) >= 1 {
			for _, ext := range *bv {
				if filepath.Ext(value) == ext {
					return true, nil
				}
			}
			return false, nil
		} else {
			return false, fmt.Errorf("Error IsInRange: BoundaryValue is invalid")
		}
	}
	return false, nil
}

var SOUND_EXTS = []string{".wav", ".ogg", ".flac", ".mp3"}
var IMAGE_EXTS = []string{".bmp", ".png", ".jpg", ".jpeg", ".gif"}
var MOVIE_EXTS = []string{".mpg", ".mpeg", ".wmv", ".avi", ".mp4", ".webm", ".m4v", ".m1v", ".m2v"}
var BMP_EXTS = append(IMAGE_EXTS, MOVIE_EXTS...)

var COMMANDS = []Command{
	Command{"player", Int, Necessary, &[]int{1, 4}},
	Command{"genre", String, Semi_necessary, nil},
	Command{"title", String, Necessary, nil},
	Command{"artist", String, Semi_necessary, nil},
	Command{"subtitle", String, Unnecessary, nil},
	Command{"subartist", String, Unnecessary, nil},
	Command{"bpm", Float, Necessary, &[]float64{0, math.MaxFloat64}},
	Command{"playlevel", Int, Semi_necessary, &[]int{0, math.MaxInt64}},
	Command{"rank", Int, Semi_necessary, &[]int{0, 4}},
	Command{"defexrank", Float, Unnecessary, &[]float64{0, math.MaxFloat64}},
	Command{"total", Float, Semi_necessary, &[]float64{0, math.MaxFloat64}},
	Command{"difficulty", Int, Semi_necessary, &[]int{1, 5}},
	Command{"stagefile", Path, Unnecessary, &IMAGE_EXTS},
	Command{"banner", Path, Unnecessary, &IMAGE_EXTS},
	Command{"backbmp", Path, Unnecessary, &IMAGE_EXTS},
	Command{"preview", Path, Unnecessary, &SOUND_EXTS},
	Command{"lntype", Int, Unnecessary, &[]int{1, 2}},
	Command{"lnobj", String, Unnecessary, &[]string{`^[0-9a-zA-Z]{2}$`}},
	Command{"lnmode", Int, Unnecessary, &[]int{1, 3}},
	Command{"volwav", Int, Unnecessary, &[]int{0, math.MaxInt64}},
	Command{"comment", String, Unnecessary, nil},
}

var NUMBERING_COMMANDS = []Command{
	Command{"wav", Path, Necessary, &SOUND_EXTS},
	Command{"bmp", Path, Unnecessary, &BMP_EXTS},
	Command{"bpm", Float, Unnecessary, &[]float64{0, math.MaxFloat64}},
	Command{"stop", Float, Unnecessary, &[]float64{0, math.MaxFloat64}},
	Command{"scroll", Float, Unnecessary, &[]float64{math.SmallestNonzeroFloat64, math.MaxFloat64}},
}

func main() {
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
		err := scanDirectory(path)
		if err != nil {
			fmt.Println("Error: scanDirectory error:", err.Error())
			os.Exit(1)
		}
	} else if isBmsPath(path) {
		bmsFile, err := loadBmsFile(path)
		if err != nil {
			fmt.Println("Error: loadBms error:", err.Error())
			os.Exit(1)
		}
		checkBmsFile(bmsFile)
	} else {
		fmt.Println("Error: Entered path is not bms file or directory")
		os.Exit(1)
	}
}

func scanDirectory(path string) error {
	files, _ := ioutil.ReadDir(path)

	hasBmsFile := false
	for _, f := range files {
		if isBmsPath(f.Name()) {
			hasBmsFile = true
			break
		}
	}
	if hasBmsFile {
		_, err := scanBmsDirectory(path, true)
		if err != nil {
			return err
		}
	} else {
		for _, f := range files {
			if f.IsDir() {
				err := scanDirectory(filepath.Join(path, f.Name()))
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func scanBmsDirectory(path string, isRootDir bool) (*Directory, error) {
	dir := NewDirectory(path)
	files, _ := ioutil.ReadDir(path)

	for _, f := range files {
		filePath := filepath.Join(path, f.Name())
		if !f.IsDir() || (f.IsDir() && !isRootDir) {
			if containsEnvironmentDependentRune(f.Name()) {
				fmt.Println("ERROR: This filename has environment-dependent characters:", f.Name())
			}
		}
		if isBmsPath(f.Name()) {
			var bmsfile *BmsFile
			if filepath.Ext(filePath) == ".bmson" {
				bmsfile = NewBmsFile(filePath) // TODO 仮 loadBmson作る
				//bmsfile, err := LoadBmson(bmspath)
			} else {
				if isUtf8, err := isUTF8(filePath); err == nil && isUtf8 {
					fmt.Println("ERROR: Bmsfile charset is UTF-8:", filePath)
				}
				var err error
				bmsfile, err = loadBmsFile(filePath)
				if err != nil {
					return nil, err
				}
			}
			dir.BmsFiles = append(dir.BmsFiles, *bmsfile)
		} else if f.IsDir() {
			innnerDir, err := scanBmsDirectory(filePath, false)
			if err != nil {
				return nil, err
			}
			dir.Directories = append(dir.Directories, *innnerDir)
		} else {
			dir.NonBmsFiles = append(dir.NonBmsFiles, *NewNonBmsFile(filePath))
		}
	}
	if isRootDir {
		checkBmsDirectory(dir)
	}

	return dir, nil
}

func loadBmsFile(path string) (*BmsFile, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("BMSfile open error: " + err.Error())
	}
	defer file.Close()

	const (
		initialBufSize = 10000
		maxBufSize = 1000000
	)
	scanner := bufio.NewScanner(file)
	buf := make([]byte, initialBufSize)
	scanner.Buffer(buf, maxBufSize)
	lineNumber := 0

	bmsFile := NewBmsFile(path)
	fieldBorders := []string{"HEADER FIELD", "EXPANSION FIELD", "MAIN DATA FIELD"}
	randomCommands := []string{"random", "if", "endif"}
	chmap := map[string]bool{"7k": false, "10k": false, "14k": false}
	for scanner.Scan() {
		/*if lineNumber == 0 && bytes.HasPrefix(([]byte)(scanner.Text()), []byte{0xef, 0xbb, 0xbf}) {
		  fmt.Println("Error, character code is UTF-8(BOM):", path)
		}*/
		if bytes.Equal(([]byte)(scanner.Text()), []byte{0xef, 0xbb, 0xbf}) {
			// entrust isUTF8()
			continue
		}

		trimmedText := strings.TrimSpace(scanner.Text())
		if trimmedText == "" {
			continue
		}
		line, _, err := transform.String(japanese.ShiftJIS.NewDecoder(), trimmedText)
		if err != nil {
			return nil, fmt.Errorf("ShiftJIS decode error: " + err.Error())
		}

		correctLine := false
		for _, border := range fieldBorders {
			if strings.HasPrefix(line, "*---------------------- " + border) {
				correctLine = true
				break
			}
		}
		if !correctLine {
			for _, command := range COMMANDS {
				if strings.HasPrefix(strings.ToLower(line), "#" + command.Name + " ") ||
				strings.ToLower(line) == ("#" + command.Name) {
					data := ""
					if strings.ToLower(line) != ("#" + command.Name) {
						length := utf8.RuneCountInString(command.Name) + 1
						data = strings.TrimSpace(line[length:])
					}
					val, ok := bmsFile.Header[command.Name]
					if ok {
						bmsFile.Logs = append(bmsFile.Logs, fmt.Sprintf("WARNING: #%s is duplicate: old=%s new=%s",
							strings.ToUpper(command.Name), val, data))
					}
					if !ok || (ok && data != "") { // 重複しても空文字だったら値を採用しない
						bmsFile.Header[command.Name] = data
					}
					correctLine = true
					break
				}
			}
		}
		if !correctLine {
			for _, command := range NUMBERING_COMMANDS {
				if regexp.MustCompile(`#` + command.Name + `[0-9a-z]{2} .+`).MatchString(strings.ToLower(line)) {
					data := ""
					length := utf8.RuneCountInString(command.Name) + 3
					lineCommand := strings.ToLower(line[1:length])
					if strings.ToLower(line) != ("#" + lineCommand) {
						data = strings.TrimSpace(line[length:])
					}

					replace := func(defs *[]definition) {
						isDuplicate := false
						for i, _ := range *defs {
							if (*defs)[i].Command == lineCommand {
								bmsFile.Logs = append(bmsFile.Logs, fmt.Sprintf("WARNING: #%s is duplicate: old=%s new=%s\n",
									strings.ToUpper(lineCommand), (*defs)[i].Value, data))
								if data != "" {
									(*defs)[i].Value = data
								}
								isDuplicate = true
								break
							}
						}
						if !isDuplicate {
							*defs = append(*defs, definition{lineCommand, data})
						}
					}
					if command.Name == "wav" {
						replace(&bmsFile.HeaderWav)
					} else if command.Name == "bmp" {
						replace(&bmsFile.HeaderBmp)
					} else {
						replace(&bmsFile.HeaderNumbering)
					}
					correctLine = true
					break
				}
			}
		}
		if !correctLine {
			if regexp.MustCompile(`#[0-9a-z]{5}:.+`).MatchString(strings.ToLower(line)) {
				data := strings.TrimSpace(line[7:])
				bmsFile.Pattern = append(bmsFile.Pattern, definition{strings.ToLower(line[1:6]), strings.ToLower(data)})
				correctLine = true
				chint, _ := strconv.Atoi(line[4:6])
				if (chint >= 18 && chint <= 19) || (chint >= 38 && chint <= 39) {
					chmap["7k"] = true
				} else if (chint >= 21 && chint <= 26) || (chint >= 41 && chint <= 46) {
					chmap["10k"] = true
				} else if (chint >= 28 && chint <= 29) || (chint >= 48 && chint <= 49) {
					chmap["14k"] = true
				}

				if (chint >= 11 && chint <= 19) || (chint >= 21 && chint <= 29) ||
				(chint >= 51 && chint <= 59) || (chint >= 61 && chint <= 69) { // LN TODO LN始点のみをノーツ数に加算する?
					for i := 2; i < len(data) + 1; i += 2 {
						if data[i-2:i] != "00" {
							bmsFile.TotalNotes++
						}
					}
				}
			}
		}
		if !correctLine {
			for _, command := range randomCommands {
				if strings.HasPrefix(strings.ToLower(line), "#" + command + " ") || strings.ToLower(line) == ("#" + command) {
					/*length := utf8.RuneCountInString(command) + 1
					  data := strings.TrimSpace(line[length:])*/
					bmsFile.Pattern = append(bmsFile.Pattern, definition{strings.ToLower(line[1:6]), strings.ToLower(line[7:])})
					correctLine = true
					break
				}
			}
		}
		if !correctLine {
			if regexp.MustCompile(`#[0-9]{3}sc:.+`).MatchString(strings.ToLower(line)) {
				bmsFile.Pattern = append(bmsFile.Pattern, definition{strings.ToLower(line[1:6]), strings.ToLower(line[7:])})
				correctLine = true
			}
		}

		if !correctLine {
			bmsFile.Logs = append(bmsFile.Logs, fmt.Sprintf("ERROR: Invalid line(%d): %s", lineNumber, line))
		}
		lineNumber++
	}
	if scanner.Err() != nil {
		return nil, fmt.Errorf("BMSfile scan error: " + scanner.Err().Error())
	}

	if filepath.Ext(path) == ".pms" {
		bmsFile.Keymode = 9
	} else if chmap["10k"] || chmap["14k"] {
		if chmap["7k"] || chmap["14k"] {
			bmsFile.Keymode = 14
		} else {
			bmsFile.Keymode = 10
		}
	} else if chmap["7k"] {
		bmsFile.Keymode = 7
	} else {
		bmsFile.Keymode = 5
	}

	return bmsFile, nil
}

func checkBmsFile(bmsFile *BmsFile) {
	for _, command := range COMMANDS {
		val, ok := bmsFile.Header[command.Name]
		if !ok {
			if command.Necessity != Unnecessary {
				alertLevel := "ERROR"
				if command.Necessity == Semi_necessary {
					alertLevel = "WARNING"
				}
				bmsFile.Logs = append(bmsFile.Logs, fmt.Sprintf("%s: #%s definition is missing", alertLevel, strings.ToUpper(command.Name)))
			}
		} else if val == "" {
			bmsFile.Logs = append(bmsFile.Logs, fmt.Sprintf("WARNING: #%s value is empty", strings.ToUpper(command.Name)))
		} else if isInRange, err := command.IsInRange(val); err != nil || !isInRange {
			if err != nil {
				fmt.Println("DEBUG ERROR: IsInRange return error")
			}
			bmsFile.Logs = append(bmsFile.Logs, fmt.Sprintf("ERROR: #%s has invalid value: %s", strings.ToUpper(command.Name), val))
		} else if command.Name == "rank" { // ここらへんはCommand型のCheck関数的なものに置き換えたい
			rank, _ := strconv.Atoi(val)
			if rank == 0 {
				bmsFile.Logs = append(bmsFile.Logs, "NOTICE: #RANK is 0(VERY HARD)")
			} else if rank == 1 {
				bmsFile.Logs = append(bmsFile.Logs, "NOTICE: #RANK is 1(HARD)")
			} else if rank == 4 {
				bmsFile.Logs = append(bmsFile.Logs, "NOTICE: #RANK is 4(VERY EASY)")
			}
		} else if command.Name == "total" {
			total, _ := strconv.ParseFloat(val, 64)
			if total < 100 {
				bmsFile.Logs = append(bmsFile.Logs, "WARNING: #TOTAL is under 100: " + val)
			} else {
				defaultTotal := bmsFile.CalculateDefaultTotal()
				overRate := 1.6
				totalPerNotes := total / float64(bmsFile.TotalNotes) // TODO 適切な基準値は？
				if total > defaultTotal * overRate && totalPerNotes > 0.35 {
					bmsFile.Logs = append(bmsFile.Logs, fmt.Sprintf("NOTICE: #TOTAL is too high(TotalNotes=%d): %s", bmsFile.TotalNotes, val))
				} else if total < defaultTotal / overRate && totalPerNotes < 0.2 {
					bmsFile.Logs = append(bmsFile.Logs, fmt.Sprintf("NOTICE: #TOTAL is too low(TotalNotes=%d): %s", bmsFile.TotalNotes, val))
				}
			}
		} else if command.Name == "defexrank" {
			bmsFile.Logs = append(bmsFile.Logs, "NOTICE: #DEFEXRANK is defined: " + val)
		}
	}

	title, ok1 := bmsFile.Header["title"]
	subtitle, ok2 := bmsFile.Header["subtitle"]
	if ok1 && ok2 && subtitle != "" {
		if strings.HasSuffix(title, subtitle) {
			bmsFile.Logs = append(bmsFile.Logs, "WARNING: #TITLE and #SUBTITLE have same text: "+subtitle)
		}
	}

	// Check invalid value of Numbering commands
	numberingCommandDefs := append(append(bmsFile.HeaderWav, bmsFile.HeaderBmp...), bmsFile.HeaderNumbering...)
	defined := make([]bool, len(NUMBERING_COMMANDS))
	var hasNoWavExtDef definition
	for _, def := range numberingCommandDefs {
		for i, nc := range NUMBERING_COMMANDS {
			if strings.HasPrefix(def.Command, nc.Name) {
				defined[i] = true
				if def.Value == "" {
					bmsFile.Logs = append(bmsFile.Logs, fmt.Sprintf("WARNING: #%s value is empty", strings.ToUpper(def.Command)))
				} else if isInRange, err := nc.IsInRange(def.Value); err != nil || !isInRange {
					if err != nil {
						bmsFile.Logs = append(bmsFile.Logs, "DEBUG ERROR: IsInRange return error: " + def.Value)
					}
					bmsFile.Logs = append(bmsFile.Logs, fmt.Sprintf("ERROR: #%s has invalid value: %s", strings.ToUpper(def.Command), def.Value))
				} else if strings.HasPrefix(def.Command, "wav") && filepath.Ext(def.Value) != ".wav" && hasNoWavExtDef.Command == "" {
					hasNoWavExtDef = def
				}
			}
		}
	}
	for i, d := range defined {
		if !d && NUMBERING_COMMANDS[i].Necessity != Unnecessary {
			alertLevel := "ERROR"
			if NUMBERING_COMMANDS[i].Necessity == Semi_necessary {
				alertLevel = "WARNING"
			}
			bmsFile.Logs = append(bmsFile.Logs, fmt.Sprintf("%s: #%sxx definition is missing", alertLevel, strings.ToUpper(NUMBERING_COMMANDS[i].Name)))
		}
	}
	if hasNoWavExtDef.Command != "" {
		bmsFile.Logs = append(bmsFile.Logs, fmt.Sprintf("NOTICE: #%s has file not .wav extension: %s",
			strings.ToUpper(hasNoWavExtDef.Command), hasNoWavExtDef.Value)) // TODO 複数の場合はその旨も出力する？
	}

	if bmsFile.TotalNotes == 0 {
		bmsFile.Logs = append(bmsFile.Logs, "ERROR: TotalNotes is 0")
	}

	// Check defined bmp/wav is used
	matchChannel := func(ch string, channels *[]string) bool {
		for _, c := range *channels {
			if ch == c {
				return true
			}
		}
		return false
	}
	checkDefinedObjIsUsed := func(commandName string, definitions *[]definition, channels *[]string, ignoreObj string) {
		usedObjs := map[string]bool{}
		for _, def := range *definitions {
			usedObjs[def.Command[3:5]] = false
		}
		undefinedObjs := []string{}
		for _, def := range bmsFile.Pattern {
			if matchChannel(def.Command[3:5], channels) {
				for i := 2; i < len(def.Value) + 1; i += 2 {
					obj := def.Value[i-2:i]
					if obj != "00" {
						if _, ok := usedObjs[obj]; ok {
							usedObjs[obj] = true
						} else {
							undefinedObjs = append(undefinedObjs, obj)
						}
					}
				}
			}
		}
		if len(undefinedObjs) > 0 {
			for _, obj := range removeDuplicate(undefinedObjs) {
				if obj != ignoreObj {
					bmsFile.Logs = append(bmsFile.Logs, fmt.Sprintf("WARNING: Used %s object is undefined: %s",
						commandName, strings.ToUpper(obj)))
				}
			}
		}
		for _, def := range *definitions {
			if !usedObjs[def.Command[3:5]] {
				bmsFile.Logs = append(bmsFile.Logs, fmt.Sprintf("WARNING: Defined %s object is not used: %s(%s)",
					commandName, strings.ToUpper(def.Command[3:5]), def.Value))
			}
		}
	}
	bmpChannels := []string{"04", "06", "07"/*, "0A"*/}
	checkDefinedObjIsUsed("BMP", &bmsFile.HeaderBmp, &bmpChannels, "")
	wavChannels := []string{"01", "11", "12", "13", "14", "15", "16", "17", "18", "19",
		"21", "22", "23", "24", "25", "26", "27", "28", "29",
		"31", "32", "33", "34", "35", "36", "37", "38", "39",
		"41", "42", "43", "44", "45", "46", "47", "48", "49",
		"51", "52", "53", "54", "55", "56", "57", "58", "59",
		"61", "62", "63", "64", "65", "66", "67", "68", "69",
	}
	checkDefinedObjIsUsed("WAV", &bmsFile.HeaderWav, &wavChannels, strings.ToLower(bmsFile.Header["lnobj"]))

	// Check WAV duplicate
	beforeMeasure := 0
	sameMeasureWavs := []definition{}
	maxValueLength := 0
	for patternIndex, def := range bmsFile.Pattern {
		measure, err := strconv.Atoi(def.Command[:3])
		if err != nil {
			bmsFile.Logs = append(bmsFile.Logs, fmt.Sprintf("DEBUG ERROR: Measure atoi error: %s", err.Error()))
		} else if measure < beforeMeasure { // TODO IFを考慮
			bmsFile.Logs = append(bmsFile.Logs, fmt.Sprintf("WARNING: Measure order is not ascending: prev=%d next=%d", beforeMeasure, measure))
		} else {
			if measure == beforeMeasure {
				if def.Value != "00" && matchChannel(def.Command[3:5], &wavChannels) {
					sameMeasureWavs = append(sameMeasureWavs, def)
					if len(def.Value) / 2 > maxValueLength {
						maxValueLength = len(def.Value) / 2
					}
				}
			}
			if measure != beforeMeasure || patternIndex == len(bmsFile.Pattern)-1 {
				if len(sameMeasureWavs) > 0 {
					wavIndexs := make([]int, len(sameMeasureWavs))
					valFrac := func(frac [2]int) float64 {
						return float64(frac[0]) / float64(frac[1])
					}
					for fIndexPos := [2]int{0, 1}; valFrac(fIndexPos) < 1.0; {
						sameTimingObjs := []string{}
						fMin := [2]int{1, 1}
						for i, wav := range sameMeasureWavs {
							fObjPos := [2]int{wavIndexs[i], len(wav.Value) / 2}
							if valFrac(fObjPos) >= 1.0 {
								continue
							}
							objWavValue := wav.Value[wavIndexs[i]*2:wavIndexs[i]*2+2]
							if valFrac(fObjPos) == valFrac(fIndexPos) && objWavValue != "00" {
								sameTimingObjs = append(sameTimingObjs, objWavValue)
							}
							if valFrac(fObjPos) <= valFrac(fIndexPos) {
								wavIndexs[i]++
								fNextObjPos := [2]int{wavIndexs[i], len(wav.Value) / 2}
								if valFrac(fNextObjPos) < valFrac(fMin) {
									fMin = fNextObjPos
								}
							}
						}
						if len(sameTimingObjs) > 0 {
							duplicates := []string{}
							objCounts := map[string]int{}
							for _, obj := range sameTimingObjs {
								if objCounts[obj] == 1 {
									duplicates = append(duplicates, obj)
								}
								objCounts[obj]++
							}
							if len(duplicates) > 0 {
								fp := [2]int{fIndexPos[0], fIndexPos[1]}
								fp = reduceFraction(fp)
								for _, dup := range duplicates {
									bmsFile.Logs = append(bmsFile.Logs, fmt.Sprintf("WARNING: Used WAV is duplicate(#%3d, %d/%d): %s * %d",
										beforeMeasure, fp[0], fp[1], strings.ToUpper(dup), objCounts[dup]))
								}
							}
						}
						fIndexPos = fMin
					}
					sameMeasureWavs = []definition{}
					maxValueLength = 0
				}
				beforeMeasure = measure
			}
		}
	}

	if len(bmsFile.Logs) > 0 {
		fmt.Printf("# BmsFile checklog: %s\n", bmsFile.Path)
		for _, log := range bmsFile.Logs {
			fmt.Println(" " + log)
		}
	}
}

func checkBmsDirectory(bmsDir *Directory) {
	var logs []string

	withoutExtPath := func(path string) string {
		return path[:len(path) - len(filepath.Ext(path))]
	}
	relativePathFromBmsRoot := func(path string) string {
		relativePath := filepath.Clean(path)
		rootDirPath := filepath.Clean(bmsDir.Path)
		if rootDirPath != "." {
			relativePath = filepath.Clean(relativePath[len(rootDirPath) + 1:])
		}
		return relativePath
	}
	containsInNonBmsFiles := func(path string, exts *[]string) bool {
		contains := false // 拡張子補完の対称ファイルを全てUsedにする
		definedFilePath := filepath.Clean(path)
		for i, _ := range bmsDir.NonBmsFiles { // TODO フォルダ内フォルダ対応(単次元化する？)
			realFilePath := relativePathFromBmsRoot(bmsDir.NonBmsFiles[i].Path)
			if definedFilePath == realFilePath {
				bmsDir.NonBmsFiles[i].Used = true
				contains = true
			} else if exts != nil && hasExts(realFilePath, exts) &&
			withoutExtPath(definedFilePath) == withoutExtPath(realFilePath) {
				bmsDir.NonBmsFiles[i].Used = true
				contains = true
			}
		}
		return contains
	}
	noFileLog := func(level, path, command, value string) string {
		return fmt.Sprintf("%s: Defined file does not exist(%s): #%s %s", level,
		relativePathFromBmsRoot(path), strings.ToUpper(command), value)
	}

	for _, bmsFile := range bmsDir.BmsFiles {
		checkBmsFile(&bmsFile)

		// Check defined files existance
		imageCommands := []string{"stagefile", "banner", "backbmp"}
		for _, command := range imageCommands {
			val, ok := bmsFile.Header[command]
			if ok && val != "" {
				if !containsInNonBmsFiles(val, nil) {
					logs = append(logs, noFileLog("WARNING", bmsFile.Path, command, val))
				}
			}
		}

		if val, ok := bmsFile.Header["preview"]; ok && val != "" {
			if !containsInNonBmsFiles(val, &SOUND_EXTS) {
				logs = append(logs, noFileLog("WARNING", bmsFile.Path, "preview", val))
			}
		}

		checkDefinedFileExists := func(defs *[]definition, exts *[]string) {
			for _, def := range *defs {
				if def.Value != "" {
					if !containsInNonBmsFiles(def.Value, exts) {
						logs = append(logs, noFileLog("ERROR", bmsFile.Path, def.Command, def.Value))
					}
				}
			}
		}
		checkDefinedFileExists(&bmsFile.HeaderWav, &SOUND_EXTS)
		//checkDefinedFileExists(&bmsFile.HeaderBmp, &MOVIE_EXTS)
		for _, def := range bmsFile.HeaderBmp {
			if def.Value != "" {
				exts := &IMAGE_EXTS
				if hasExts(def.Value, &MOVIE_EXTS) {
					exts = &MOVIE_EXTS
				}
				if !containsInNonBmsFiles(def.Value, exts) {
					logs = append(logs, noFileLog("ERROR", bmsFile.Path, def.Command, def.Value))
				}
			}
		}
	}

	unityCommands := []string{"stagefile", "banner", "backbmp", "preview"}
	isNotUnified := make([]bool, len(unityCommands))
	values := make([][]string, len(unityCommands))
	for i, bmsFile := range bmsDir.BmsFiles {
		for j, uc := range unityCommands {
			values[j] = append(values[j], bmsFile.Header[uc])
			if i > 0 && values[j][i-1] != bmsFile.Header[uc] {
				isNotUnified[j] = true
			}
		}
	}
	for index, uc := range unityCommands {
		if isNotUnified[index] {
			strs := []string{}
			for j, bmsFile := range bmsDir.BmsFiles {
				strs = append(strs, fmt.Sprintf("  %s: %s", relativePathFromBmsRoot(bmsFile.Path), values[index][j]))
			}
			logs = append(logs, fmt.Sprintf("WARNING: #%s is not unified: \n%s", strings.ToUpper(uc), strings.Join(strs, "\n")))
		}
	}

	isPreview := func(path string) bool {
		for _, ext := range SOUND_EXTS {
			if relativePathFromBmsRoot(path) == ("preview" + ext) { // TODO preview*.でも再生される？
				return true
			}
		}
		return false
	}
	ignoreExts := []string{".txt", ".zip", ".rar", ".lzh", ".7z"}
	for _, nonBmsFile := range bmsDir.NonBmsFiles {
		if !nonBmsFile.Used && !hasExts(nonBmsFile.Path, &ignoreExts) && !isPreview(nonBmsFile.Path) {
			logs = append(logs, "NOTICE: This file is not used: " + relativePathFromBmsRoot(nonBmsFile.Path))
		}
	}

	if len(logs) > 0 {
		dirPath := filepath.Clean(bmsDir.Path)
		if dirPath == "." {
			dirPath, _ = filepath.Abs(dirPath)
			dirPath = filepath.Base(dirPath)
		}
		fmt.Printf("## BmsDirectory checklog: %s\n", dirPath)
		for _, log := range logs {
			fmt.Println(" " + log)
		}
	}
}

func hasExts(path string, exts *[]string) bool {
	for _, ext := range *exts {
		if strings.ToLower(ext) == strings.ToLower(filepath.Ext(path)) {
			return true
		}
	}
	return false
}

func isBmsPath(path string) bool {
	bmsExts := []string{".bms", ".bme", ".bml", ".pms", ".bmson"}
	return hasExts(path, &bmsExts)
}

func containsEnvironmentDependentRune(text string) bool {
	return !regexp.MustCompile(`^[0-9a-zA-Z !#$%&'\(\)\-\+\^@\[\];,\.=~\{\}_]+$`).MatchString(text)
}

func isUTF8(path string) (bool, error) {
	fBytes, err := ioutil.ReadFile(path)
	if err != nil {
		return false, err
	}

	det := chardet.NewTextDetector()
	detResult, err := det.DetectBest(fBytes)
	if err != nil {
		fmt.Println("Detect error:", err.Error())
		return false, err
	}
	if detResult.Charset == "UTF-8" {
		return true, nil
	}
	return false, nil
}

func removeDuplicate(args []string) []string {
	result := make([]string, 0, len(args))
	encounterd := map[string]bool{}
	for i := 0; i < len(args); i++ {
		if !encounterd[args[i]] {
			encounterd[args[i]] = true
			result = append(result, args[i])
		}
	}
	return result
}

func reduceFraction(fraction [2]int) [2]int {
	numerator := big.NewInt(int64(fraction[0]))
	denominator := big.NewInt(int64(fraction[1]))
	gcd := big.NewInt(1)
	gcd = gcd.GCD(nil, nil, numerator, denominator)
	if gcd.Int64() > 1 {
		fraction[0] /= int(gcd.Int64())
		fraction[1] /= int(gcd.Int64())
		fraction = reduceFraction(fraction)
	}
	return fraction
}
