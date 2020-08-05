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
	"sort"
	"math"
	"math/big"
	"io/ioutil"
	"path/filepath"
	"unicode/utf8"

	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/transform"
	"github.com/saintfish/chardet"

	"./diff"
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
func newDirectory(path string) *Directory {
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
	BmsWavObjs *[]bmsObj
	BmsBmpObjs *[]bmsObj
	Keymode int // 5, 7, 9, 10, 14, 24, 48
	TotalNotes int
	Logs []string
}
func newBmsFile(path string) *BmsFile {
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
func (bf BmsFile) calculateDefaultTotal() float64 {
	tn := float64(bf.TotalNotes)
	if bf.Keymode >= 24 {
		return math.Max(300.0, 7.605 * (tn + 100.0) / (0.01 * tn + 6.5))
	} else {
		return math.Max(260.0, 7.605 * tn / (0.01 * tn + 6.5))
	}
}
func fileName(index string, defs *[]definition) string { // WAV or BMPを選ばせる？
	for _, def := range *defs { // TODO 高速化？ソートしてO(logn)にする？
		if def.Command[3:] == index {
			return def.Value
		}
	}
	return ""
}
func (bf BmsFile) wavFileName(value string) string {
  return fileName(value, &bf.HeaderWav)
}
func (bf BmsFile) bmpFileName(value string) string {
  return fileName(value, &bf.HeaderBmp)
}


type NonBmsFile struct {
	File
	Used bool
}
func newNonBmsFile(path string) *NonBmsFile {
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
func (c Command) isInRange(value string) (bool, error) {
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
			return false, fmt.Errorf("Error isInRange: BoundaryValue is invalid")
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
			return false, fmt.Errorf("Error isInRange: BoundaryValue is invalid")
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
			return false, fmt.Errorf("Error isInRange: BoundaryValue is invalid")
		}
		return false, nil
	case Path:
		if bv, ok := c.BoundaryValue.(*[]string); ok && len(*bv) >= 1 {
			for _, ext := range *bv {
				if strings.ToLower(filepath.Ext(value)) == ext {
					return true, nil
				}
			}
			return false, nil
		} else {
			return false, fmt.Errorf("Debug error isInRange: BoundaryValue is invalid")
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

var BMP_CHANNNELS = []string{"04", "06", "07"}
var WAV_CHANNNELS = []string{"01",
	"11", "12", "13", "14", "15", "16", "17", "18", "19",
	"21", "22", "23", "24", "25", "26", "27", "28", "29",
	"31", "32", "33", "34", "35", "36", "37", "38", "39",
	"41", "42", "43", "44", "45", "46", "47", "48", "49",
	"51", "52", "53", "54", "55", "56", "57", "58", "59",
	"61", "62", "63", "64", "65", "66", "67", "68", "69",
}
var NOTE_CHANNNELS = []string{
	"11", "12", "13", "14", "15", "16", "17", "18", "19",
	"21", "22", "23", "24", "25", "26", "27", "28", "29",
	"51", "52", "53", "54", "55", "56", "57", "58", "59",
	"61", "62", "63", "64", "65", "66", "67", "68", "69",
}
var LN_CHANNNELS = []string{
	"51", "52", "53", "54", "55", "56", "57", "58", "59",
	"61", "62", "63", "64", "65", "66", "67", "68", "69",
}
func matchChannel(ch string, channels *[]string) bool {
	for _, c := range *channels {
		if ch == c {
			return true
		}
	}
	return false
}

type fraction struct {
	Numerator int
	Denominator int
}
func (f fraction) value() float64 {
	return float64(f.Numerator) / float64(f.Denominator)
}
func (f *fraction) reduce() {
	bigNme := big.NewInt(int64(f.Numerator))
	bigDnm := big.NewInt(int64(f.Denominator))
	gcd := big.NewInt(1)
	gcd = gcd.GCD(nil, nil, bigNme, bigDnm)
	if gcd.Int64() > 1 {
		f.Numerator /= int(gcd.Int64())
		f.Denominator /= int(gcd.Int64())
		f.reduce()
	}
}

type bmsObj struct {
	Channel string
	Measure int
	Position fraction
	Value int // 36進数→10進数
	IsLNEnd bool
}
func (bo bmsObj) time() float64 {
	return float64(bo.Measure) + bo.Position.value()
}
func (bo bmsObj) value36() string {
	return strconv.FormatInt(int64(bo.Value), 36)
}
func (bf BmsFile) bmsObjs() (*[]bmsObj, *[]bmsObj) {
	wavObjs := []bmsObj{}
	bmpObjs := []bmsObj{}
	ongoingLNs := map[int]bool{}
	for _, def := range bf.Pattern {
		channel := def.Command[3:]
		chint, _ := strconv.Atoi(def.Command[3:])
		measure, _ := strconv.Atoi(def.Command[:3])
		for i := 0; i < len(def.Value)/2; i++ {
			valStr := def.Value[i*2:i*2+2]
			val, _ := strconv.ParseInt(valStr, 36, 64)
			if val == 0 {
				continue
			}
			pos := fraction{i, len(def.Value)/2}
			obj := bmsObj{Channel: channel, Measure: measure, Position: pos, Value: int(val)}
			if matchChannel(channel, &WAV_CHANNNELS) {
				if valStr == bf.Header["lnobj"] {
					obj.IsLNEnd = true
					ongoingLNs[chint+40] = false
				}	else if matchChannel(channel, &LN_CHANNNELS) {
					if ongoingLNs[chint] {
						obj.IsLNEnd = true
						ongoingLNs[chint] = false
					} else {
						ongoingLNs[chint] = true
					}
				}
				wavObjs = append(wavObjs, obj)
			} else if matchChannel(channel, &BMP_CHANNNELS) {
				bmpObjs = append(bmpObjs, obj)
			}
		}
	}
	sortObjs := func(objs *[]bmsObj) {
		sort.Slice(*objs, func(i, j int) bool { return (*objs)[i].Value < (*objs)[j].Value })
		sort.SliceStable(*objs, func(i, j int) bool { return (*objs)[i].time() < (*objs)[j].time() })
	}
	sortObjs(&wavObjs)
	sortObjs(&bmpObjs)
	return &wavObjs, &bmpObjs
}

type note struct {
	Channel string
	Value string
}
type patternMoment struct {
	Measure int
	Position fraction
	Objs []note
}
type patternIterator struct {
	patterns *[]definition
	targetChannels *[]string
	index int
	measure int
	position *fraction
	sameMeasureLanes *[]definition
	laneIndexs []int
}
func newPattenIterator(patterns *[]definition, targetChannels *[]string) patternIterator {
	pi := patternIterator{}
	pi.patterns = patterns
	pi.targetChannels = targetChannels
	return pi
}
func (pi *patternIterator) next() (moment *patternMoment, logs *[]string) {
	logs = &[]string{}
	for ; moment == nil; {
		if pi.sameMeasureLanes == nil {
			pi.sameMeasureLanes = &[]definition{}
			for ; len(*pi.sameMeasureLanes) == 0; {
				if pi.index >= len(*pi.patterns) {
					return nil, logs
				}
				var beforeMeasure int
				for initIndex := pi.index; pi.index < len(*pi.patterns); pi.index++ {
					def := (*pi.patterns)[pi.index]
					pi.measure, _ = strconv.Atoi(def.Command[:3])
					if pi.index == initIndex {
						beforeMeasure = pi.measure
					}
					if pi.measure < beforeMeasure { // TODO #IFを考慮
						*logs = append(*logs, fmt.Sprintf("WARNING: Measure order is not ascending: prev=%d next=%d", beforeMeasure, pi.measure))
					}
					if pi.measure == beforeMeasure {
						if def.Value != "00" && (pi.targetChannels == nil || matchChannel(def.Command[3:5], pi.targetChannels)) {
							*pi.sameMeasureLanes = append(*pi.sameMeasureLanes, def)
						}
					}
					if pi.measure != beforeMeasure {
						pi.measure = beforeMeasure
						break
					}
					beforeMeasure = pi.measure
				}
			}
		}

		if pi.laneIndexs == nil {
			pi.laneIndexs = make([]int, len(*pi.sameMeasureLanes))
		}
		if pi.position == nil {
			pi.position = &fraction{0, 1}
		}
		sameTimingNotes := []note{}
		for ; len(sameTimingNotes) == 0 && pi.position.value() < 1.0; {
			minNextObjPos := fraction{1, 1}
			for i, lane := range *pi.sameMeasureLanes {
				objPos := fraction{pi.laneIndexs[i], len(lane.Value) / 2}
				if objPos.value() >= 1.0 {
					continue
				}
				objValue := lane.Value[pi.laneIndexs[i]*2:pi.laneIndexs[i]*2+2]
				if objPos.value() == pi.position.value() && objValue != "00" {
					sameTimingNotes = append(sameTimingNotes, note{Channel: lane.Command[3:5], Value: objValue})
				}

				if objPos.value() == pi.position.value() { // objPos.value() < pi.position.value() is impossible
					pi.laneIndexs[i]++
				}
				nextObjPos := fraction{pi.laneIndexs[i], len(lane.Value) / 2}
				for ; pi.laneIndexs[i] < len(lane.Value) / 2; pi.laneIndexs[i]++ {
					nextObjPos = fraction{pi.laneIndexs[i], len(lane.Value) / 2}
					nextObjValue := lane.Value[pi.laneIndexs[i]*2:pi.laneIndexs[i]*2+2]
					if nextObjValue != "00" {
						break
					}
				}
				if nextObjPos.value() < minNextObjPos.value() {
					minNextObjPos = nextObjPos
				}
			}
			if len(sameTimingNotes) > 0 {
				moment = &patternMoment{Measure: pi.measure, Position: *pi.position, Objs: sameTimingNotes}
			}
			*pi.position = minNextObjPos
		}
		if pi.position.value() >= 1.0 {
			pi.position = nil
			pi.sameMeasureLanes = nil
			pi.laneIndexs = nil
		}
	}
	return moment, logs
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
	dir := newDirectory(path)
	files, _ := ioutil.ReadDir(path)

	for _, f := range files {
		filePath := filepath.Join(path, f.Name())
		if !f.IsDir() || (f.IsDir() && !isRootDir) { // TODO チェック対象をbms関連ファイルと.txtのみにする
			if containsEnvironmentDependentRune(f.Name()) {
				fmt.Println("ERROR: This filename has environment-dependent characters:", f.Name())
			}
		}
		if isRootDir && isBmsPath(f.Name()) {
			var bmsfile *BmsFile
			if filepath.Ext(filePath) == ".bmson" {
				bmsfile = newBmsFile(filePath) // TODO 仮 loadBmson作る
				//bmsfile, err := LoadBmson(bmspath)
			} else {
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
			dir.NonBmsFiles = append(dir.NonBmsFiles, innnerDir.NonBmsFiles...)
			dir.Directories = append(dir.Directories, innnerDir.Directories...)
		} else {
			dir.NonBmsFiles = append(dir.NonBmsFiles, *newNonBmsFile(filePath))
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

	containsMultibyteRune := false
	bmsFile := newBmsFile(path)
	randomCommands := []string{"random", "if", "endif"}
	for lineNumber := 0; scanner.Scan(); lineNumber++ {
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
		if !containsMultibyteRune && len(line) != utf8.RuneCountInString(line) {
			containsMultibyteRune = true
		}

		correctLine := false
		if strings.HasPrefix(line, "*") { // skip comment line
			correctLine = true
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
								bmsFile.Logs = append(bmsFile.Logs, fmt.Sprintf("WARNING: #%s is duplicate: old=%s new=%s",
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
			if regexp.MustCompile(`#[0-9]{3}[0-9a-z]{2}:.+`).MatchString(strings.ToLower(line)) {
				data := strings.TrimSpace(line[7:])
				if len(data) % 2 == 0 && regexp.MustCompile(`^[0-9a-zA-Z]+$`).MatchString(data) { // TODO invalid lineではなくinvalid valueとして出力したい
					bmsFile.Pattern = append(bmsFile.Pattern, definition{strings.ToLower(line[1:6]), strings.ToLower(data)})
					correctLine = true
				}
			}
		}
		if !correctLine {
			if regexp.MustCompile(`#[0-9]{3}sc:.+`).MatchString(strings.ToLower(line)) { // SCROLL Speed
				bmsFile.Pattern = append(bmsFile.Pattern, definition{strings.ToLower(line[1:6]), strings.ToLower(line[7:])})
				correctLine = true
			}
		}
		if !correctLine {
			for _, command := range randomCommands {
				if strings.HasPrefix(strings.ToLower(line), "#" + command + " ") || strings.ToLower(line) == ("#" + command) {
					// TODO: #IF対応
					/*length := utf8.RuneCountInString(command) + 1
					data := strings.TrimSpace(line[length:])
					bmsFile.Pattern = append(bmsFile.Pattern, definition{command, strings.ToLower(length)})*/
					correctLine = true
					break
				}
			}
		}

		if !correctLine {
			bmsFile.Logs = append(bmsFile.Logs, fmt.Sprintf("ERROR: Invalid line(%d): %s", lineNumber, line))
		}
	}
	if scanner.Err() != nil {
		return nil, fmt.Errorf("BMSfile scan error: " + scanner.Err().Error())
	}

	if isUtf8, err := isUTF8(path); err == nil && isUtf8 {
		if containsMultibyteRune {
			bmsFile.Logs = append(bmsFile.Logs, "ERROR: Bmsfile charset is UTF-8 and contains multibyte characters")
		} else {
			bmsFile.Logs = append(bmsFile.Logs, "WARNING: Bmsfile charset is UTF-8")
		}
	}

	chmap := map[string]bool{"7k": false, "10k": false, "14k": false}
	lnCount := 0
	for _, pattern := range bmsFile.Pattern {
		chint, _ := strconv.Atoi(pattern.Command[3:5])
		if (chint >= 18 && chint <= 19) || (chint >= 38 && chint <= 39) {
			chmap["7k"] = true
		} else if (chint >= 21 && chint <= 26) || (chint >= 41 && chint <= 46) {
			chmap["10k"] = true
		} else if (chint >= 28 && chint <= 29) || (chint >= 48 && chint <= 49) {
			chmap["14k"] = true
		}

		if (chint >= 11 && chint <= 19) || (chint >= 21 && chint <= 29) ||
		(chint >= 51 && chint <= 59) || (chint >= 61 && chint <= 69) {
			for i := 2; i < len(pattern.Value) + 1; i += 2 {
				if obj := pattern.Value[i-2:i]; obj != "00" && obj != bmsFile.Header["lnobj"] {
					if (chint >= 51 && chint <= 59) || (chint >= 61 && chint <= 69) {
						lnCount++
					} else {
						bmsFile.TotalNotes++
					}
				}
			}
		}
	}
	bmsFile.TotalNotes += lnCount / 2

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

	bmsFile.BmsWavObjs, bmsFile.BmsBmpObjs = bmsFile.bmsObjs()

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
		} else if isInRange, err := command.isInRange(val); err != nil || !isInRange {
			if err != nil {
				fmt.Println("DEBUG ERROR: isInRange return error")
			}
			bmsFile.Logs = append(bmsFile.Logs, fmt.Sprintf("ERROR: #%s has invalid value: %s", strings.ToUpper(command.Name), val))
		} else if command.Name == "rank" { // TODO ここらへんはCommand型のCheck関数的なものに置き換えたい？
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
				defaultTotal := bmsFile.calculateDefaultTotal()
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
		} else if command.Name == "lntype" {
			if val == "2" {
				bmsFile.Logs = append(bmsFile.Logs, "WARNING: #LNTYPE 2(MGQ) is deprecated")
			}
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
				} else if isInRange, err := nc.isInRange(def.Value); err != nil || !isInRange {
					if err != nil {
						bmsFile.Logs = append(bmsFile.Logs, "DEBUG ERROR: isInRange return error: " + def.Value)
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
	checkDefinedObjIsUsed("BMP", &bmsFile.HeaderBmp, &BMP_CHANNNELS, "")
	checkDefinedObjIsUsed("WAV", &bmsFile.HeaderWav, &WAV_CHANNNELS, strings.ToLower(bmsFile.Header["lnobj"]))

	// Check WAV duplicate
	pi := newPattenIterator(&bmsFile.Pattern, &WAV_CHANNNELS)
	for moment, logs := pi.next(); moment != nil; moment, logs = pi.next() {
		if logs != nil && len(*logs) > 0 { // TODO 小節番号に関するログは事前にまとめて出しておく？
			bmsFile.Logs = append(bmsFile.Logs, *logs...)
		}
		duplicates := []string{}
		objCounts := map[string]int{}
		for _, obj := range moment.Objs {
			if bmsFile.wavFileName(strings.ToUpper(obj.Value)) == "" {
				continue
			}
			if objCounts[obj.Value] == 1 {
				duplicates = append(duplicates, obj.Value)
			}
			objCounts[obj.Value]++
		}
		if len(duplicates) > 0 {
			fp := fraction{moment.Position.Numerator, moment.Position.Denominator}
			fp.reduce()
			for _, dup := range duplicates {
				bmsFile.Logs = append(bmsFile.Logs, fmt.Sprintf("WARNING: Used WAV is duplicate(#%03d,%d/%d): %s (%s) * %d",
					moment.Measure, fp.Numerator, fp.Denominator, strings.ToUpper(dup), bmsFile.wavFileName(strings.ToUpper(dup)), objCounts[dup]))
			}
		}
	}

	// Check end of LN exists
	type LNstart struct {
		Measure int
		Position fraction
		Value string
	}
	ongoingLNs := map[string]*LNstart{}
	pi = newPattenIterator(&bmsFile.Pattern, &NOTE_CHANNNELS)
	for moment, _ := pi.next(); moment != nil; moment, _ = pi.next() {
		for _, obj := range moment.Objs {
			chint, _ := strconv.Atoi(obj.Channel)
			if (chint >= 51 && chint <= 59) || (chint >= 61 && chint <= 69) {
				if ongoingLNs[obj.Channel] == nil {
					ongoingLNs[obj.Channel] = &LNstart{moment.Measure, moment.Position, obj.Value}
				} else if bmsFile.Header["lnobj"] == "" {
					if ongoingLNs[obj.Channel].Value != obj.Value {
						bmsFile.Logs = append(bmsFile.Logs, fmt.Sprintf("WARNING: LN start and end are not equal: %s(#%d,%d/%d) -> %s(#%d,%d/%d)",
							strings.ToUpper(ongoingLNs[obj.Channel].Value), ongoingLNs[obj.Channel].Measure, ongoingLNs[obj.Channel].Position.Numerator, ongoingLNs[obj.Channel].Position.Denominator,
							strings.ToUpper(obj.Value), moment.Measure, moment.Position.Numerator, moment.Position.Denominator))
					}
					ongoingLNs[obj.Channel] = nil
				}
			} else if (chint >= 11 && chint <= 19) || (chint >= 21 && chint <= 29) {
				lnCh := strconv.Itoa(chint+40)
				if ongoingLNs[lnCh] != nil {
					if obj.Value == bmsFile.Header["lnobj"] {
						ongoingLNs[lnCh] = nil
					} else {
						bmsFile.Logs = append(bmsFile.Logs, fmt.Sprintf("ERROR: Normal note is in LN: %s(#%d,%d/%d) in %s(#%d,%d/%d)",
							strings.ToUpper(obj.Value), moment.Measure, moment.Position.Numerator, moment.Position.Denominator,
							strings.ToUpper(ongoingLNs[lnCh].Value), ongoingLNs[lnCh].Measure, ongoingLNs[lnCh].Position.Numerator, ongoingLNs[lnCh].Position.Denominator))
					}
				}
			}
		}
	}
	for _, lnStart := range ongoingLNs { // TODO ソートして表示する？
		if lnStart != nil {
			bmsFile.Logs = append(bmsFile.Logs, fmt.Sprintf("ERROR: LN is not finished: %s(#%d,%d/%d)",
				strings.ToUpper(lnStart.Value), lnStart.Measure, lnStart.Position.Numerator, lnStart.Position.Denominator))
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
		for i, _ := range bmsDir.NonBmsFiles {
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
			if regexp.MustCompile(`^preview.*` + ext + `$`).MatchString(relativePathFromBmsRoot(path)) {
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
	for _, dir := range bmsDir.Directories {
		if len(dir.BmsFiles) == 0 && len(dir.NonBmsFiles) == 0 && len(dir.Directories) == 0 {
			logs = append(logs, "NOTICE: This directory is empty: " + relativePathFromBmsRoot(dir.Path))
		}
	}

	// diff
	// TODO ファイルごとの比較ではなく、定義・配置ごとの比較にする？
	missingLog := func(path, val string) string {
		return fmt.Sprintf(" Missing(%s): %s", path, val)
	}
	for i := 0; i < len(bmsDir.BmsFiles); i++ {
		for j := i+1; j < len(bmsDir.BmsFiles); j++ {
			diffDefs := func(label string, iBmsFile, jBmsFile *BmsFile) {
				var iDefs, jDefs *[]definition
				switch label {
				case "WAV":
					iDefs, jDefs = &iBmsFile.HeaderWav, &jBmsFile.HeaderWav
				case "BMP":
					iDefs, jDefs = &iBmsFile.HeaderBmp, &jBmsFile.HeaderBmp
				}
				iDefStrs, jDefStrs := []string{}, []string{}
				for _, def := range *iDefs {
					iDefStrs = append(iDefStrs, fmt.Sprintf("#%s %s", strings.ToUpper(def.Command), def.Value))
				}
				for _, def := range *jDefs {
					jDefStrs = append(jDefStrs, fmt.Sprintf("#%s %s", strings.ToUpper(def.Command), def.Value))
				}
				ed, ses := diff.Onp(&iDefStrs, &jDefStrs)
				if ed > 0 {
					logs = append(logs, fmt.Sprintf("WARNING: There are %d differences in %s definitions: %s %s",
						ed, label, iBmsFile.Path, jBmsFile.Path))
					ii, jj := 0, 0
				  for _, r := range ses {
				    switch r {
				    case '=':
				      ii++
				      jj++
				    case '+':
							logs = append(logs, missingLog(iBmsFile.Path, jDefStrs[jj]))
				      jj++
				    case '-':
							logs = append(logs, missingLog(jBmsFile.Path, iDefStrs[ii]))
				      ii++
				    }
				  }
				}
			}
			diffDefs("WAV", &bmsDir.BmsFiles[i], &bmsDir.BmsFiles[j])
			diffDefs("BMP", &bmsDir.BmsFiles[i], &bmsDir.BmsFiles[j])

			diffObjs := func(label string, iBmsFile, jBmsFile *BmsFile) {
				toString := func(bo bmsObj, defs *[]definition) string {
					val := bo.value36()
					if len(val) == 1 {
						val = "0" + val
					}
					return fmt.Sprintf("#%03d %s (%d/%d) #%s%s (%s)",
						bo.Measure, bo.Channel, bo.Position.Numerator, bo.Position.Denominator, label, strings.ToUpper(val), fileName(val, defs))
				}
				_logs := []string{}
				var iObjs, jObjs *[]bmsObj
				var iDefs, jDefs *[]definition
				switch label {
				case "WAV":
					iObjs, jObjs = iBmsFile.BmsWavObjs, jBmsFile.BmsWavObjs
					iDefs, jDefs = &iBmsFile.HeaderWav, &jBmsFile.HeaderWav
				case "BMP":
					iObjs, jObjs = iBmsFile.BmsBmpObjs, jBmsFile.BmsBmpObjs
					iDefs, jDefs = &iBmsFile.HeaderBmp, &jBmsFile.HeaderBmp
				}
				ii, jj := 0, 0
				for ; ii < len(*iObjs) && jj < len(*jObjs); {
					iObj, jObj := (*iObjs)[ii], (*jObjs)[jj]
					if iObj.IsLNEnd {
						ii++
						continue
					}
					if jObj.IsLNEnd {
						jj++
						continue
					}
					if iObj.time() == jObj.time() && iObj.Value == jObj.Value {
						ii++
						jj++
					} else if iObj.time() < jObj.time() || (iObj.time() == jObj.time() && iObj.Value < jObj.Value) {
						if fileName(iObj.value36(), iDefs) != "" {
							_logs = append(_logs, missingLog(jBmsFile.Path, toString(iObj, iDefs)))
						}
						ii++
					} else {
						if fileName(jObj.value36(), jDefs) != "" {
							_logs = append(_logs, missingLog(iBmsFile.Path, toString(jObj, jDefs)))
						}
						jj++
					}
				}
				for ; ii < len(*iObjs)-1; ii++ {
					iObj := (*iObjs)[ii]
					if !iObj.IsLNEnd && fileName(iObj.value36(), iDefs) != "" {
						_logs = append(_logs, missingLog(jBmsFile.Path, toString(iObj, iDefs)))
					}
				}
				for ; jj < len(*jObjs)-1; jj++ {
					jObj := (*jObjs)[jj]
					if !jObj.IsLNEnd && fileName(jObj.value36(), jDefs) != "" {
						_logs = append(_logs, missingLog(iBmsFile.Path, toString(jObj, jDefs)))
					}
				}
				if len(_logs) > 0 {
					s := fmt.Sprintf("WARNING: There are %d differences in %s objects: %s, %s",
						len(_logs), label, iBmsFile.Path, jBmsFile.Path)
					logs = append(append(logs, s), _logs...)
				}
			}
			diffObjs("WAV", &bmsDir.BmsFiles[i], &bmsDir.BmsFiles[j])
			diffObjs("BMP", &bmsDir.BmsFiles[i], &bmsDir.BmsFiles[j])
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
