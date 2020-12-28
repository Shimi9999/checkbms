package checkbms

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"fmt"
	"io/ioutil"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/saintfish/chardet"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/transform"

	"github.com/Shimi9999/checkbms/audio"
	"github.com/Shimi9999/checkbms/diff"
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
	Logs        Logs
}

func newDirectory(path string) *Directory {
	var d Directory
	d.Path = path
	return &d
}
func (d Directory) LogString(base bool) string {
	str := ""
	if len(d.Logs) > 0 {
		dirPath := filepath.Clean(d.Path)
		if dirPath == "." {
			dirPath, _ = filepath.Abs(dirPath)
			dirPath = filepath.Base(dirPath)
		} else if base {
			dirPath = filepath.Base(d.Path)
		}
		str += fmt.Sprintf("## BmsDirectory checklog: %s\n", dirPath)
		str += d.Logs.String()
	}
	return str
}

type definition struct {
	Command string
	Value   string
}

type indexedDefinition struct {
	CommandName string
	Index       string
	Value       string
}

func (id indexedDefinition) command() string {
	return id.CommandName + id.Index
}

func (id indexedDefinition) equalCommand(command string) bool {
	return command == id.command()
}

type objType int

const (
	Wav objType = iota + 1
	Bmp
	Mine
	Bpm
	ExtendedBpm
	Stop
	Scroll
)

func (ot objType) string() string {
	switch ot {
	case Wav:
		return "WAV"
	case Bmp:
		return "BMP"
	case Mine:
		return "WAV"
	case Bpm:
		return "BPM"
	case ExtendedBpm:
		return "BPM"
	case Stop:
		return "STOP"
	case Scroll:
		return "SCROLL"
	}
	return ""
}

type BmsFile struct {
	File
	Header             map[string]string
	HeaderWav          []indexedDefinition
	HeaderBmp          []indexedDefinition
	HeaderExtendedBpm  []indexedDefinition
	HeaderStop         []indexedDefinition
	HeaderScroll       []indexedDefinition
	BmsWavObjs         []bmsObj
	BmsBmpObjs         []bmsObj
	BmsMineObjs        []bmsObj
	BmsBpmObjs         []bmsObj
	BmsExtendedBpmObjs []bmsObj
	BmsStopObjs        []bmsObj
	BmsScrollObjs      []bmsObj
	BmsMeasureLengths  []measureLength
	Keymode            int // 5, 7, 9, 10, 14, 24, 48
	TotalNotes         int
	Sha256             string
	Logs               Logs
}

func newBmsFile(path string) *BmsFile {
	var bf BmsFile
	bf.Path = path
	bf.Header = make(map[string]string)
	return &bf
}
func (bf BmsFile) calculateDefaultTotal() float64 {
	tn := float64(bf.TotalNotes)
	if bf.Keymode >= 24 {
		return math.Max(300.0, 7.605*(tn+100.0)/(0.01*tn+6.5))
	} else {
		return math.Max(260.0, 7.605*tn/(0.01*tn+6.5))
	}
}
func (bf BmsFile) headerIndexedDefs(t objType) []indexedDefinition {
	switch t {
	case Wav:
		return bf.HeaderWav
	case Bmp:
		return bf.HeaderBmp
	case ExtendedBpm:
		return bf.HeaderExtendedBpm
	case Stop:
		return bf.HeaderStop
	case Scroll:
		return bf.HeaderScroll
	}
	return nil
}
func (bf BmsFile) bmsObjs(t objType) []bmsObj {
	switch t {
	case Wav:
		return bf.BmsWavObjs
	case Bmp:
		return bf.BmsBpmObjs
	case Mine:
		return bf.BmsMineObjs
	case Bpm:
		return bf.BmsBpmObjs
	case ExtendedBpm:
		return bf.BmsExtendedBpmObjs
	case Stop:
		return bf.BmsStopObjs
	case Scroll:
		return bf.BmsScrollObjs
	}
	return nil
}
func (bf BmsFile) definedValue(t objType, index string) string {
	for _, def := range bf.headerIndexedDefs(t) { // TODO 高速化？ソートしてO(logn)にする？
		if def.Index == index {
			return def.Value
		}
	}
	return ""
}
func (bf *BmsFile) sortBmsObjs() {
	sortObjs := func(objs []bmsObj) {
		sort.Slice(objs, func(i, j int) bool { return objs[i].Value < objs[j].Value })
		sort.SliceStable(objs, func(i, j int) bool { return objs[i].time() < objs[j].time() })
	}
	sortObjs(bf.BmsWavObjs)
	sortObjs(bf.BmsBmpObjs)
	sortObjs(bf.BmsMineObjs)
	sortObjs(bf.BmsBpmObjs)
	sortObjs(bf.BmsExtendedBpmObjs)
	sortObjs(bf.BmsStopObjs)
	sortObjs(bf.BmsScrollObjs)
	sort.Slice(bf.BmsMeasureLengths, func(i, j int) bool { return bf.BmsMeasureLengths[i].Measure < bf.BmsMeasureLengths[j].Measure })
}
func (bf *BmsFile) setIsLNEnd() {
	ongoingLNs := map[int]bool{}
	for i := 0; i < len(bf.BmsWavObjs); i++ {
		if bf.BmsWavObjs[i].Channel == "01" {
			continue
		}
		chint, _ := strconv.Atoi(bf.BmsWavObjs[i].Channel)
		if bf.BmsWavObjs[i].value36() == bf.Header["lnobj"] {
			bf.BmsWavObjs[i].IsLNEnd = true
			ongoingLNs[chint+40] = false
		} else if matchChannel(bf.BmsWavObjs[i].Channel, LN_CHANNELS) {
			if ongoingLNs[chint] {
				bf.BmsWavObjs[i].IsLNEnd = true
				ongoingLNs[chint] = false
			} else {
				ongoingLNs[chint] = true
			}
		}
	}
}
func (bf BmsFile) LogString(base bool) string {
	str := ""
	if len(bf.Logs) > 0 {
		path := bf.Path
		if base {
			path = filepath.Base(bf.Path)
		}
		str += fmt.Sprintf("# BmsFile checklog: %s\n", path)
		str += bf.Logs.String()
	}
	return str
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

type fraction struct {
	Numerator   int
	Denominator int
}

func (f fraction) value() float64 {
	if f.Denominator == 0 {
		return -1.0
	}
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
	ObjType  objType
	Channel  string
	Measure  int
	Position fraction
	Value    int // 36進数→10進数
	IsLNEnd  bool
}

func (bo bmsObj) time() float64 {
	return float64(bo.Measure) + bo.Position.value()
}
func (bo bmsObj) value36() string {
	val := strconv.FormatInt(int64(bo.Value), 36)
	if len(val) == 1 {
		val = "0" + val
	}
	return val
}
func (bo bmsObj) string(bmsFile *BmsFile) string {
	val := bo.value36()
	definedValue := ""
	if bmsFile != nil {
		definedValue = fmt.Sprintf(" (%s)", bmsFile.definedValue(bo.ObjType, val))
	}
	return fmt.Sprintf("#%03d %s (%d/%d) #%s%s%s",
		bo.Measure, bo.Channel, bo.Position.Numerator, bo.Position.Denominator, bo.ObjType.string(), strings.ToUpper(val), definedValue)
}

type measureLength struct {
	Measure   int
	LengthStr string
}

func (ml measureLength) length() float64 {
	length, _ := strconv.ParseFloat(ml.LengthStr, 64)
	return length
}

type bmsObjsIterator struct {
	bmsObjs []bmsObj
	index   int
	time    float64
}

func newBmsObjsIterator(bmsObjs []bmsObj) *bmsObjsIterator {
	boi := bmsObjsIterator{}
	boi.bmsObjs = bmsObjs
	if len(boi.bmsObjs) > 0 {
		boi.time = boi.bmsObjs[0].time()
	}
	return &boi
}
func (boi *bmsObjsIterator) next() (momentObjs []bmsObj) {
	momentObjs = []bmsObj{}
	for ; boi.index < len(boi.bmsObjs); boi.index++ {
		if boi.time == boi.bmsObjs[boi.index].time() {
			momentObjs = append(momentObjs, boi.bmsObjs[boi.index])
		} else {
			boi.time = boi.bmsObjs[boi.index].time()
			break
		}
	}
	return momentObjs
}

type AlertLevel string

const (
	Error   = AlertLevel("ERROR")
	Warning = AlertLevel("WARNING")
	Notice  = AlertLevel("NOTICE")
	Debug   = AlertLevel("DEBUG ERROR")
)

type Log struct {
	Level   AlertLevel
	Message string
	SubLogs []string
}

func newLog(level AlertLevel, message string) *Log {
	var log Log
	log.Level = level
	log.Message = message
	return &log
}
func (log Log) String() string {
	str := string(log.Level) + ": " + log.Message
	for _, subLog := range log.SubLogs {
		str += "\n  " + subLog
	}
	return str
}

type Logs []Log

func (logs *Logs) addNewLog(level AlertLevel, message string) {
	*logs = append(*logs, *newLog(level, message))
}
func (logs Logs) String() string {
	var str string
	for i, log := range logs {
		if i > 0 {
			str += "\n"
		}
		str += log.String()
	}
	return str
}

type CommandType int

const (
	Int CommandType = iota + 1
	Float
	String
	Path
)

type CommandNecessity int

const (
	Necessary CommandNecessity = iota + 1
	Semi_necessary
	Unnecessary
)

type Command struct {
	Name          string
	Type          CommandType
	Necessity     CommandNecessity
	BoundaryValue interface{} // Intなら0と100,-10とnilとか。Stringなら*とか(多分不要)。Pathなら許容する拡張子.oog、.wav
	//Check func(string) []string // 引数の値に対してチェックをしてエラーメッセージ(string)のスライスを返す関数
}

func (c Command) isInRange(value string) (bool, error) {
	invalidError := fmt.Errorf("Error isInRange: BoundaryValue is invalid")
	switch c.Type {
	case Int:
		intValue, err := strconv.Atoi(value)
		if err != nil {
			return false, err
		}
		if bv, ok := c.BoundaryValue.([]int); ok && len(bv) >= 2 {
			if intValue >= bv[0] && intValue <= bv[1] {
				return true, nil
			} else {
				return false, nil
			}
		} else {
			return false, invalidError
		}
	case Float:
		floatValue, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return false, err
		}
		if bv, ok := c.BoundaryValue.([]float64); ok && len(bv) >= 2 {
			if floatValue >= bv[0] && floatValue <= bv[1] {
				return true, nil
			} else {
				return false, nil
			}
		} else {
			return false, invalidError
		}
	case String:
		if c.BoundaryValue == nil {
			return true, nil
		}
		if bv, ok := c.BoundaryValue.([]string); ok && len(bv) >= 1 {
			if regexp.MustCompile(bv[0]).MatchString(value) { // 条件複数個にする？単数にする？
				return true, nil
			}
		} else {
			return false, invalidError
		}
		return false, nil
	case Path:
		if bv, ok := c.BoundaryValue.([]string); ok && len(bv) >= 1 {
			for _, ext := range bv {
				if strings.ToLower(filepath.Ext(value)) == ext {
					return true, nil
				}
			}
			return false, nil
		} else {
			return false, invalidError
		}
	}
	return false, nil
}

var AUDIO_EXTS = []string{".wav", ".ogg", ".flac", ".mp3"}
var IMAGE_EXTS = []string{".bmp", ".png", ".jpg", ".jpeg", ".gif"}
var MOVIE_EXTS = []string{".mpg", ".mpeg", ".wmv", ".avi", ".mp4", ".webm", ".m4v", ".m1v", ".m2v"}
var BMP_EXTS = append(IMAGE_EXTS, MOVIE_EXTS...)

var COMMANDS = []Command{
	Command{"player", Int, Necessary, []int{1, 4}},
	Command{"genre", String, Semi_necessary, nil},
	Command{"title", String, Necessary, nil},
	Command{"artist", String, Semi_necessary, nil},
	Command{"subtitle", String, Unnecessary, nil},
	Command{"subartist", String, Unnecessary, nil},
	Command{"bpm", Float, Necessary, []float64{math.SmallestNonzeroFloat64, math.MaxFloat64}},
	Command{"playlevel", Int, Semi_necessary, []int{0, math.MaxInt64}},
	Command{"rank", Int, Semi_necessary, []int{0, 4}},
	Command{"defexrank", Float, Unnecessary, []float64{0, math.MaxFloat64}},
	Command{"total", Float, Semi_necessary, []float64{0, math.MaxFloat64}},
	Command{"difficulty", Int, Semi_necessary, []int{0, 5}},
	Command{"stagefile", Path, Unnecessary, IMAGE_EXTS},
	Command{"banner", Path, Unnecessary, IMAGE_EXTS},
	Command{"backbmp", Path, Unnecessary, IMAGE_EXTS},
	Command{"preview", Path, Unnecessary, AUDIO_EXTS},
	Command{"lntype", Int, Unnecessary, []int{1, 2}},
	Command{"lnobj", String, Unnecessary, []string{`^[0-9a-zA-Z]{2}$`}},
	Command{"lnmode", Int, Unnecessary, []int{1, 3}},
	Command{"volwav", Int, Unnecessary, []int{0, math.MaxInt64}},
	Command{"comment", String, Unnecessary, nil},
}

var INDEXED_COMMANDS = []Command{
	Command{"wav", Path, Necessary, AUDIO_EXTS},
	Command{"bmp", Path, Unnecessary, BMP_EXTS},
	Command{"bpm", Float, Unnecessary, []float64{math.SmallestNonzeroFloat64, math.MaxFloat64}},
	Command{"stop", Float, Unnecessary, []float64{math.SmallestNonzeroFloat64, math.MaxFloat64}},
	Command{"scroll", Float, Unnecessary, []float64{-math.MaxFloat64, math.MaxFloat64}},
}

var BMP_CHANNELS = []string{"04", "06", "07"}
var NORMALNOTE_CHANNELS = []string{
	"11", "12", "13", "14", "15", "16", "17", "18", "19",
	"21", "22", "23", "24", "25", "26", "27", "28", "29",
}
var INVISIBLENOTE_CHANNELS = []string{
	"31", "32", "33", "34", "35", "36", "37", "38", "39",
	"41", "42", "43", "44", "45", "46", "47", "48", "49",
}
var LN_CHANNELS = []string{
	"51", "52", "53", "54", "55", "56", "57", "58", "59",
	"61", "62", "63", "64", "65", "66", "67", "68", "69",
}
var NOTE_CHANNELS = append(NORMALNOTE_CHANNELS, LN_CHANNELS...)
var WAV_CHANNELS = append(append(append([]string{"01"}, NORMALNOTE_CHANNELS...), INVISIBLENOTE_CHANNELS...), LN_CHANNELS...)
var MINE_CHANNELS = []string{
	"d1", "d2", "d3", "d4", "d5", "d6", "d7", "d8", "d9",
	"e1", "e2", "e3", "e4", "e5", "e6", "e7", "e8", "e9",
}
var MEASURE_CHANNELS = []string{"02"}
var BPM_CHANNELS = []string{"03"}
var EXTENDEDBPM_CHANNELS = []string{"08"}
var STOP_CHANNELS = []string{"09"}
var SCROLL_CHANNELS = []string{"sc"}

func matchChannel(ch string, channels []string) bool {
	for _, c := range channels {
		if ch == c {
			return true
		}
	}
	return false
}

func ScanDirectory(path string) ([]Directory, error) {
	bmsDirs := []Directory{}
	if IsBmsDirectory(path) {
		bmsDir, err := scanBmsDirectory(path, true)
		if err != nil {
			return nil, err
		}
		bmsDirs = append(bmsDirs, *bmsDir)
	} else {
		files, err := ioutil.ReadDir(path)
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			if f.IsDir() {
				_bmsDirs, err := ScanDirectory(filepath.Join(path, f.Name()))
				if err != nil {
					return nil, err
				}
				bmsDirs = append(bmsDirs, _bmsDirs...)
			}
		}
	}

	return bmsDirs, nil
}

func scanBmsDirectory(path string, isRootDir bool) (*Directory, error) {
	dir := newDirectory(path)
	files, _ := ioutil.ReadDir(path)

	for _, f := range files {
		filePath := filepath.Join(path, f.Name())
		if isRootDir && IsBmsFile(f.Name()) {
			var bmsfile *BmsFile
			if filepath.Ext(filePath) == ".bmson" {
				// TODO loadBmson作る
				// bmsfile, err := LoadBmson(bmspath)
				continue
			} else {
				var err error
				bmsfile, err = ScanBmsFile(filePath)
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

	return dir, nil
}

func ScanBmsFile(path string) (*BmsFile, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("BMSfile open error: " + err.Error())
	}
	defer file.Close()

	const (
		initialBufSize = 10000
		maxBufSize     = 1000000
	)
	scanner := bufio.NewScanner(file)
	buf := make([]byte, initialBufSize)
	scanner.Buffer(buf, maxBufSize)

	hasUtf8Bom := false
	hasMultibyteRune := false
	bmsFile := newBmsFile(path)
	randomCommands := []string{"random", "if", "endif"}
	for lineNumber := 0; scanner.Scan(); lineNumber++ {
		if lineNumber == 0 && bytes.HasPrefix(([]byte)(scanner.Text()), []byte{0xef, 0xbb, 0xbf}) {
			hasUtf8Bom = true
		}
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
			return nil, fmt.Errorf("Shift-JIS decode error: " + err.Error())
		}
		if !hasMultibyteRune && containsMultibyteRune(line) {
			hasMultibyteRune = true
		}

		if strings.HasPrefix(line, "*") || strings.HasPrefix(line, "%") { // skip comment/meta line
			goto correctLine
		}
		if strings.HasPrefix(line, "#") {
			for _, command := range COMMANDS {
				if strings.HasPrefix(strings.ToLower(line), "#"+command.Name+" ") ||
					strings.ToLower(line) == ("#"+command.Name) {
					data := ""
					if strings.ToLower(line) != ("#" + command.Name) {
						length := len(command.Name) + 1
						data = strings.TrimSpace(line[length:])
					}
					val, ok := bmsFile.Header[command.Name]
					if ok {
						bmsFile.Logs.addNewLog(Warning, fmt.Sprintf("#%s is duplicate: old=%s new=%s",
							strings.ToUpper(command.Name), val, data))
					}
					if !ok || (ok && data != "") { // 重複しても空文字だったら値を採用しない
						bmsFile.Header[command.Name] = data
					}
					goto correctLine
				}
			}
			for _, command := range INDEXED_COMMANDS {
				if regexp.MustCompile(`#` + command.Name + `[0-9a-z]{2} .+`).MatchString(strings.ToLower(line)) {
					data := ""
					length := len(command.Name) + 3
					lineCommand := strings.ToLower(line[1:length])
					if strings.ToLower(line) != ("#" + lineCommand) {
						data = strings.TrimSpace(line[length:])
					}

					replace := func(defs *[]indexedDefinition) {
						isDuplicate := false
						for i := range *defs {
							if (*defs)[i].equalCommand(lineCommand) {
								bmsFile.Logs.addNewLog(Warning, fmt.Sprintf("#%s is duplicate: old=%s new=%s",
									strings.ToUpper(lineCommand), (*defs)[i].Value, data))
								if data != "" {
									(*defs)[i].Value = data
								}
								isDuplicate = true
								break
							}
						}
						if !isDuplicate {
							*defs = append(*defs, indexedDefinition{CommandName: lineCommand[:len(lineCommand)-2], Index: lineCommand[len(lineCommand)-2:], Value: data})
						}
					}
					switch command.Name {
					case "wav":
						replace(&bmsFile.HeaderWav)
					case "bmp":
						replace(&bmsFile.HeaderBmp)
					case "bpm":
						replace(&bmsFile.HeaderExtendedBpm)
					case "stop":
						replace(&bmsFile.HeaderStop)
					case "scroll":
						replace(&bmsFile.HeaderScroll)
					}
					goto correctLine
				}
			}
			if regexp.MustCompile(`#[0-9]{3}[0-9a-z]{2}:.+`).MatchString(strings.ToLower(line)) {
				measure, _ := strconv.Atoi(line[1:4])
				channel := strings.ToLower(line[4:6])
				data := strings.TrimSpace(line[7:])
				if channel == "02" {
					if regexp.MustCompile(`^\d+(?:\.\d+)?$`).MatchString(data) {
						bmsFile.BmsMeasureLengths = append(bmsFile.BmsMeasureLengths, measureLength{Measure: measure, LengthStr: data})
						goto correctLine
					}
				} else {
					var channelType objType
					if matchChannel(channel, WAV_CHANNELS) {
						channelType = Wav
					} else if matchChannel(channel, BMP_CHANNELS) {
						channelType = Bmp
					} else if matchChannel(channel, MINE_CHANNELS) {
						channelType = Mine
					} else if matchChannel(channel, BPM_CHANNELS) {
						channelType = Bpm
					} else if matchChannel(channel, EXTENDEDBPM_CHANNELS) {
						channelType = ExtendedBpm
					} else if matchChannel(channel, STOP_CHANNELS) {
						channelType = Stop
					} else if matchChannel(channel, SCROLL_CHANNELS) {
						channelType = Scroll
					}

					if channelType != 0 && len(data)%2 == 0 && regexp.MustCompile(`^[0-9a-zA-Z]+$`).MatchString(data) {
						for i := 0; i < len(data)/2; i++ {
							valStr := data[i*2 : i*2+2]
							val, _ := strconv.ParseInt(valStr, 36, 64)
							if val == 0 {
								continue
							}
							pos := fraction{i, len(data) / 2}
							obj := bmsObj{ObjType: channelType, Channel: channel, Measure: measure, Position: pos, Value: int(val)}
							switch channelType {
							case Wav:
								bmsFile.BmsWavObjs = append(bmsFile.BmsWavObjs, obj)
							case Bmp:
								bmsFile.BmsBmpObjs = append(bmsFile.BmsBmpObjs, obj)
							case Mine:
								bmsFile.BmsMineObjs = append(bmsFile.BmsMineObjs, obj)
							case Bpm:
								bmsFile.BmsBpmObjs = append(bmsFile.BmsBpmObjs, obj)
							case ExtendedBpm:
								bmsFile.BmsExtendedBpmObjs = append(bmsFile.BmsExtendedBpmObjs, obj)
							case Stop:
								bmsFile.BmsStopObjs = append(bmsFile.BmsStopObjs, obj)
							case Scroll:
								bmsFile.BmsScrollObjs = append(bmsFile.BmsScrollObjs, obj)
							}
						}
						goto correctLine
					}
				}
			}
			for _, command := range randomCommands {
				if strings.HasPrefix(strings.ToLower(line), "#"+command+" ") || strings.ToLower(line) == ("#"+command) {
					// TODO: #IF対応
					goto correctLine
				}
			}
		}

		bmsFile.Logs.addNewLog(Error, fmt.Sprintf("Invalid line(%d): %s", lineNumber, line))

	correctLine:
	}
	if scanner.Err() != nil {
		return nil, fmt.Errorf("BMSfile scan error: " + scanner.Err().Error())
	}

	if _, err = file.Seek(0, 0); err != nil {
		return nil, fmt.Errorf("BMSfile Seek error: " + err.Error())
	}
	fullFile, err := ioutil.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("BMSfile ReadAll error: " + err.Error())
	}
	bmsFile.Sha256 = fmt.Sprintf("%x", sha256.Sum256(fullFile))

	bmsFile.sortBmsObjs()
	bmsFile.setIsLNEnd()

	isUtf8 := hasUtf8Bom
	if !isUtf8 {
		var err error
		isUtf8, err = isUTF8(path)
		if err != nil {
			isUtf8 = false
		}
	}
	if isUtf8 {
		if hasMultibyteRune {
			bmsFile.Logs.addNewLog(Error, "Bmsfile charset is UTF-8, not Shift-JIS, and contains multibyte characters")
		} else {
			bmsFile.Logs.addNewLog(Notice, "Bmsfile charset is UTF-8, not Shift-JIS")
		}
	}

	chmap := map[string]bool{"7k": false, "10k": false, "14k": false}
	lnCount := 0
	for _, obj := range bmsFile.BmsWavObjs {
		if obj.Channel == "01" {
			continue
		}
		chint, _ := strconv.Atoi(obj.Channel)
		if (chint >= 18 && chint <= 19) || (chint >= 38 && chint <= 39) {
			chmap["7k"] = true
		} else if (chint >= 21 && chint <= 26) || (chint >= 41 && chint <= 46) {
			chmap["10k"] = true
		} else if (chint >= 28 && chint <= 29) || (chint >= 48 && chint <= 49) {
			chmap["14k"] = true
		}

		if (chint >= 11 && chint <= 19) || (chint >= 21 && chint <= 29) ||
			(chint >= 51 && chint <= 59) || (chint >= 61 && chint <= 69) {
			if obj.value36() != bmsFile.Header["lnobj"] {
				if (chint >= 51 && chint <= 59) || (chint >= 61 && chint <= 69) {
					lnCount++
				} else {
					bmsFile.TotalNotes++
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

	return bmsFile, nil
}

func CheckBmsFile(bmsFile *BmsFile) {
	for _, command := range COMMANDS {
		val, ok := bmsFile.Header[command.Name]
		if !ok {
			if command.Necessity != Unnecessary {
				alertLevel := Error
				if command.Necessity == Semi_necessary {
					alertLevel = Warning
				}
				bmsFile.Logs.addNewLog(alertLevel, fmt.Sprintf("#%s definition is missing", strings.ToUpper(command.Name)))
			}
		} else if val == "" {
			bmsFile.Logs.addNewLog(Warning, fmt.Sprintf("#%s value is empty", strings.ToUpper(command.Name)))
		} else if isInRange, err := command.isInRange(val); err != nil || !isInRange {
			if err != nil {
				fmt.Println("DEBUG ERROR: isInRange return error")
			}
			bmsFile.Logs.addNewLog(Error, fmt.Sprintf("#%s has invalid value: %s", strings.ToUpper(command.Name), val))
		} else if command.Name == "rank" { // TODO ここらへんはCommand型のCheck関数的なものに置き換えたい？
			rank, _ := strconv.Atoi(val)
			if rank == 0 {
				bmsFile.Logs.addNewLog(Notice, "#RANK is 0(VERY HARD)")
			} else if rank == 1 {
				bmsFile.Logs.addNewLog(Notice, "#RANK is 1(HARD)")
			} else if rank == 4 {
				bmsFile.Logs.addNewLog(Notice, "#RANK is 4(VERY EASY)")
			}
		} else if command.Name == "total" {
			total, _ := strconv.ParseFloat(val, 64)
			if total < 100 {
				bmsFile.Logs.addNewLog(Warning, "#TOTAL is under 100: "+val)
			} else {
				defaultTotal := bmsFile.calculateDefaultTotal()
				overRate := 1.6
				totalPerNotes := total / float64(bmsFile.TotalNotes) // TODO 適切な基準値は？
				if total > defaultTotal*overRate && totalPerNotes > 0.35 {
					bmsFile.Logs.addNewLog(Notice, fmt.Sprintf("#TOTAL is very high(TotalNotes=%d): %s", bmsFile.TotalNotes, val))
				} else if total < defaultTotal/overRate && totalPerNotes < 0.2 {
					bmsFile.Logs.addNewLog(Notice, fmt.Sprintf("#TOTAL is very low(TotalNotes=%d): %s", bmsFile.TotalNotes, val))
				}
			}
		} else if command.Name == "difficulty" {
			if val == "0" {
				bmsFile.Logs.addNewLog(Warning, "#DIFFICULTY is 0(Undefined)")
			}
		} else if command.Name == "defexrank" {
			bmsFile.Logs.addNewLog(Notice, "#DEFEXRANK is defined: "+val)
		} else if command.Name == "lntype" {
			if val == "2" {
				bmsFile.Logs.addNewLog(Warning, "#LNTYPE 2(MGQ) is deprecated")
			}
		}
	}

	title, ok1 := bmsFile.Header["title"]
	subtitle, ok2 := bmsFile.Header["subtitle"]
	if ok1 && ok2 && subtitle != "" {
		if strings.HasSuffix(title, subtitle) {
			bmsFile.Logs.addNewLog(Warning, "#TITLE and #SUBTITLE have same text: "+subtitle)
		}
	}

	// Check invalid value of indexed commands
	headerIndexedDefinitions := [][]indexedDefinition{bmsFile.HeaderWav, bmsFile.HeaderBmp, bmsFile.HeaderExtendedBpm, bmsFile.HeaderStop, bmsFile.HeaderScroll}
	hasNoWavExtDefs := []indexedDefinition{}
	for i, defs := range headerIndexedDefinitions {
		if len(defs) == 0 {
			if INDEXED_COMMANDS[i].Necessity != Unnecessary {
				alertLevel := Error
				if INDEXED_COMMANDS[i].Necessity == Semi_necessary {
					alertLevel = Warning
				}
				bmsFile.Logs.addNewLog(alertLevel, fmt.Sprintf("#%sxx definition is missing", strings.ToUpper(INDEXED_COMMANDS[i].Name)))
			}
		}
		for _, def := range defs {
			if def.Value == "" {
				bmsFile.Logs.addNewLog(Warning, fmt.Sprintf("#%s value is empty", strings.ToUpper(def.command())))
			} else if isInRange, err := INDEXED_COMMANDS[i].isInRange(def.Value); err != nil || !isInRange {
				if err != nil {
					bmsFile.Logs.addNewLog(Debug, "isInRange return error: "+def.Value)
				}
				bmsFile.Logs.addNewLog(Error, fmt.Sprintf("#%s has invalid value: %s", strings.ToUpper(def.command()), def.Value))
			} else if def.CommandName == "wav" && filepath.Ext(def.Value) != ".wav" {
				hasNoWavExtDefs = append(hasNoWavExtDefs, def)
			}
		}
	}
	if len(hasNoWavExtDefs) > 0 {
		bmsFile.Logs.addNewLog(Notice, fmt.Sprintf("#WAV definition has non-.wav extension(*%d): %s %s etc...",
			len(hasNoWavExtDefs), strings.ToUpper(hasNoWavExtDefs[0].command()), hasNoWavExtDefs[0].Value))
	}

	if bmsFile.TotalNotes == 0 {
		bmsFile.Logs.addNewLog(Error, "TotalNotes is 0")
	}

	// Check wavObj exists in 0th measure
	for _, obj := range bmsFile.BmsWavObjs {
		if obj.Measure != 0 {
			break
		}
		if matchChannel(obj.Channel, NOTE_CHANNELS) {
			bmsFile.Logs.addNewLog(Warning, "Note exists in 0th measure: "+obj.string(bmsFile))
		}
	}

	// Check defined header is used, and placed obj is undefined
	checkDefinedObjIsUsed := func(t objType, definitions []indexedDefinition, objs []bmsObj, ignoreDef string, ignoreObj string) {
		usedObjs := map[string]bool{}
		for _, def := range definitions {
			usedObjs[def.Index] = false
		}
		undefinedObjs := []string{}
		for _, obj := range objs {
			if _, ok := usedObjs[obj.value36()]; ok {
				usedObjs[obj.value36()] = true
			} else {
				undefinedObjs = append(undefinedObjs, obj.value36())
			}
		}
		if len(undefinedObjs) > 0 {
			for _, obj := range removeDuplicate(undefinedObjs) {
				if obj != ignoreObj {
					bmsFile.Logs.addNewLog(Warning, fmt.Sprintf("Placed %s object is undefined: %s",
						t.string(), strings.ToUpper(obj)))
				}
			}
		}
		for _, def := range definitions {
			if !usedObjs[def.Index] && def.Index != ignoreDef {
				bmsFile.Logs.addNewLog(Warning, fmt.Sprintf("Defined %s object is not used: %s(%s)",
					t.string(), strings.ToUpper(def.Index), def.Value))
			}
		}
	}
	checkDefinedObjIsUsed(Bmp, bmsFile.HeaderBmp, bmsFile.BmsBmpObjs, "00", "") // 00:misslayer
	checkDefinedObjIsUsed(Wav, bmsFile.HeaderWav, bmsFile.BmsWavObjs, "00", strings.ToLower(bmsFile.Header["lnobj"]))
	checkDefinedObjIsUsed(Bpm, bmsFile.HeaderExtendedBpm, bmsFile.BmsExtendedBpmObjs, "", "")
	checkDefinedObjIsUsed(Stop, bmsFile.HeaderStop, bmsFile.BmsStopObjs, "", "")
	checkDefinedObjIsUsed(Scroll, bmsFile.HeaderScroll, bmsFile.BmsScrollObjs, "", "")

	// check sound of mine explosion is used
	for _, def := range bmsFile.HeaderWav {
		if def.Index == "00" && len(bmsFile.BmsMineObjs) == 0 {
			bmsFile.Logs.addNewLog(Warning, fmt.Sprintf("Defined mine explision wav(#WAV00) is not used: %s", def.Value))
		}
	}

	// Check WAV duplicate
	boi := newBmsObjsIterator(bmsFile.BmsWavObjs)
	for momentObjs := boi.next(); len(momentObjs) > 0; momentObjs = boi.next() {
		duplicates := []string{}
		objCounts := map[string]int{}
		for _, obj := range momentObjs {
			if bmsFile.definedValue(Wav, strings.ToUpper(obj.value36())) == "" {
				continue
			}
			if objCounts[obj.value36()] == 1 {
				duplicates = append(duplicates, obj.value36())
			}
			objCounts[obj.value36()]++
		}
		if len(duplicates) > 0 {
			fp := fraction{momentObjs[0].Position.Numerator, momentObjs[0].Position.Denominator}
			fp.reduce()
			for _, dup := range duplicates {
				bmsFile.Logs.addNewLog(Warning, fmt.Sprintf("Placed WAV objects are duplicate(#%03d,%d/%d): %s (%s) * %d",
					momentObjs[0].Measure, fp.Numerator, fp.Denominator, strings.ToUpper(dup), bmsFile.definedValue(Wav, strings.ToUpper(dup)), objCounts[dup]))
			}
		}
	}

	// check note overlap
	notBgmWavObjs := []bmsObj{}
	for _, obj := range bmsFile.BmsWavObjs {
		if obj.Channel != "01" {
			notBgmWavObjs = append(notBgmWavObjs, obj)
		}
	}
	allNotes := append(notBgmWavObjs, bmsFile.BmsMineObjs...)
	sort.Slice(allNotes, func(i, j int) bool { return allNotes[i].time() < allNotes[j].time() })
	boi = newBmsObjsIterator(allNotes) // TODO イテレータ内でソートすべき？
	for momentObjs := boi.next(); len(momentObjs) > 0; momentObjs = boi.next() {
		laneObjs := make([][]bmsObj, 20)
		for _, obj := range momentObjs {
			lane, _ := strconv.Atoi(obj.Channel[1:2])
			switch obj.Channel[:1] {
			case "2", "4", "6", "e":
				lane += 10
			}
			laneObjs[lane] = append(laneObjs[lane], obj)
		}
		for _, objs := range laneObjs {
			if len(objs) > 1 {
				fp := fraction{objs[0].Position.Numerator, objs[0].Position.Denominator}
				fp.reduce()
				s := ""
				for _, obj := range objs {
					s += fmt.Sprintf("[%s]%s ", obj.Channel, strings.ToUpper(obj.value36()))
				}
				bmsFile.Logs.addNewLog(Error, fmt.Sprintf("Placed notes overlap(#%03d,%d/%d): %s",
					objs[0].Measure, fp.Numerator, fp.Denominator, s))
			}
		}
	}

	// Check end of LN exists and notes in LN
	noteObjs := []bmsObj{}
	for _, obj := range bmsFile.BmsWavObjs {
		if matchChannel(obj.Channel, NOTE_CHANNELS) {
			noteObjs = append(noteObjs, obj)
		}
	}
	ongoingLNs := map[string]*bmsObj{}
	nmObjs := append(noteObjs, bmsFile.BmsMineObjs...)
	sort.Slice(nmObjs, func(i, j int) bool { return nmObjs[i].time() < nmObjs[j].time() })
	boi = newBmsObjsIterator(nmObjs)
	for momentObjs := boi.next(); len(momentObjs) > 0; momentObjs = boi.next() {
		for _, obj := range momentObjs {
			chint, _ := strconv.Atoi(obj.Channel)
			if (chint >= 51 && chint <= 59) || (chint >= 61 && chint <= 69) {
				if ongoingLNs[obj.Channel] == nil {
					ongoingLNs[obj.Channel] = &obj
				} else {
					ongoingLNs[obj.Channel] = nil
				}
			} else if (chint >= 11 && chint <= 19) || (chint >= 21 && chint <= 29) {
				lnCh := strconv.Itoa(chint + 40)
				if ongoingLNs[lnCh] != nil {
					if obj.value36() == bmsFile.Header["lnobj"] {
						ongoingLNs[lnCh] = nil
					} else {
						bmsFile.Logs.addNewLog(Error, fmt.Sprintf("Normal note is in LN: %s(#%03d %s %d/%d) in %s(#%03d %s %d/%d)",
							strings.ToUpper(obj.value36()), momentObjs[0].Measure, lnCh, momentObjs[0].Position.Numerator, momentObjs[0].Position.Denominator,
							strings.ToUpper(ongoingLNs[lnCh].value36()), ongoingLNs[lnCh].Measure, lnCh, ongoingLNs[lnCh].Position.Numerator, ongoingLNs[lnCh].Position.Denominator))
					}
				}
			} else if obj.Channel[0] == 'd' || obj.Channel[0] == 'e' { // Mine
				chint, _ := strconv.Atoi(obj.Channel[1:])
				if obj.Channel[0] == 'd' {
					chint += 50
				} else {
					chint += 60
				}
				lnCh := strconv.Itoa(chint)
				if ongoingLNs[lnCh] != nil {
					bmsFile.Logs.addNewLog(Error, fmt.Sprintf("Mine note is in LN: %s(#%03d %s %d/%d) in %s(#%03d %s %d/%d)",
						strings.ToUpper(obj.value36()), momentObjs[0].Measure, lnCh, momentObjs[0].Position.Numerator, momentObjs[0].Position.Denominator,
						strings.ToUpper(ongoingLNs[lnCh].value36()), ongoingLNs[lnCh].Measure, lnCh, ongoingLNs[lnCh].Position.Numerator, ongoingLNs[lnCh].Position.Denominator))
				}
			}
		}
	}
	ongoingLNsSlice := []*bmsObj{}
	for _, ln := range ongoingLNs {
		if ln != nil {
			ongoingLNsSlice = append(ongoingLNsSlice, ln)
		}
	}
	sort.Slice(ongoingLNsSlice, func(i, j int) bool { return ongoingLNsSlice[i].time() < ongoingLNsSlice[j].time() })
	for _, lnStart := range ongoingLNsSlice {
		bmsFile.Logs.addNewLog(Error, fmt.Sprintf("LN is not finished: %s(#%d,%d/%d)",
			strings.ToUpper(lnStart.value36()), lnStart.Measure, lnStart.Position.Numerator, lnStart.Position.Denominator))
	}

	// count no keysound moments
	momentCount := 0
	noWavMomentCount := 0
	var noWavObjs []bmsObj
	boi = newBmsObjsIterator(noteObjs)
	for momentObjs := boi.next(); len(momentObjs) > 0; momentObjs = boi.next() {
		momentCount++
		ok := false
		for _, momObj := range momentObjs {
			for _, wav := range bmsFile.HeaderWav {
				if momObj.value36() == wav.Index {
					ok = true
					break
				}
			}
			if ok {
				break
			}
		}
		if !ok {
			noWavMomentCount++
			noWavObjs = append(noWavObjs, momentObjs[0])
		}
	}
	if len(noWavObjs) > 0 {
		bmsFile.Logs.addNewLog(Warning, fmt.Sprintf("No keysound moment exists: %.1f%%(%d/%d)",
			float64(noWavMomentCount)/float64(momentCount)*100, noWavMomentCount, momentCount))
		/*for _, obj := range noWavObjs {
			fmt.Printf("#%d %d/%d, ", obj.Measure, obj.Position.Numerator, obj.Position.Denominator)
		}
		fmt.Printf("\n")*/
	}

	// check bpm value
	for _, obj := range bmsFile.BmsBpmObjs {
		_, err := strconv.ParseInt(obj.value36(), 16, 64)
		if err != nil {
			bmsFile.Logs.addNewLog(Error, fmt.Sprintf("BPM object has invalid value: %s",
				fmt.Sprintf("#%03d (%d/%d) %s",
					obj.Measure, obj.Position.Numerator, obj.Position.Denominator, strings.ToUpper(obj.value36()))))
		}
	}

	// check measure length
	duplicateMeasureLength := []measureLength{}
	for i, mlen := range bmsFile.BmsMeasureLengths {
		if mlen.length() <= 0 {
			bmsFile.Logs.addNewLog(Error, fmt.Sprintf("#%03d measure length has invalid value: %s", mlen.Measure, mlen.LengthStr))
		}
		duplicateMeasureLength = append(duplicateMeasureLength, mlen)
		if i == len(bmsFile.BmsMeasureLengths)-1 || mlen.Measure != bmsFile.BmsMeasureLengths[i+1].Measure {
			if len(duplicateMeasureLength) > 1 {
				lens := duplicateMeasureLength[0].LengthStr
				for j := 1; j < len(duplicateMeasureLength); j++ {
					lens += ", " + duplicateMeasureLength[j].LengthStr
				}
				bmsFile.Logs.addNewLog(Warning, fmt.Sprintf("#%03d measure length is duplicate: %s", mlen.Measure, lens))
			}
			duplicateMeasureLength = []measureLength{}
		}
	}
}

func CheckBmsDirectory(bmsDir *Directory, doDiffCheck bool) {
	withoutExtPath := func(path string) string {
		return path[:len(path)-len(filepath.Ext(path))]
	}
	relativePathFromBmsRoot := func(path string) string {
		relativePath := filepath.Clean(path)
		rootDirPath := filepath.Clean(bmsDir.Path)
		if rootDirPath != "." {
			relativePath = filepath.Clean(relativePath[len(rootDirPath)+1:])
		}
		return relativePath
	}
	relativeToLower := func(path string) string {
		relative := strings.ToLower(relativePathFromBmsRoot(path))
		return path[:len(path)-len(relative)] + relative
	}
	containsInNonBmsFiles := func(path string, exts []string) bool {
		contains := false // 拡張子補完の対称ファイルを全てUsedにする
		definedFilePath := filepath.Clean(strings.ToLower(path))
		for i := range bmsDir.NonBmsFiles {
			realFilePath := relativePathFromBmsRoot(relativeToLower(bmsDir.NonBmsFiles[i].Path))
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
	noFileMessage := func(path, command, value string) string {
		return fmt.Sprintf("Defined file does not exist(%s): #%s %s",
			relativePathFromBmsRoot(path), strings.ToUpper(command), value)
	}

	for i, bmsFile := range bmsDir.BmsFiles {
		CheckBmsFile(&bmsDir.BmsFiles[i])

		// check defined files existance
		imageCommands := []string{"stagefile", "banner", "backbmp"}
		for _, command := range imageCommands {
			val, ok := bmsFile.Header[command]
			if ok && val != "" {
				if !containsInNonBmsFiles(val, nil) {
					bmsDir.Logs.addNewLog(Warning, noFileMessage(bmsFile.Path, command, val))
				}
			}
		}

		if val, ok := bmsFile.Header["preview"]; ok && val != "" {
			if !containsInNonBmsFiles(val, AUDIO_EXTS) {
				bmsDir.Logs.addNewLog(Warning, noFileMessage(bmsFile.Path, "preview", val))
			}
		}

		for _, def := range bmsFile.HeaderWav {
			if def.Value != "" {
				if !containsInNonBmsFiles(def.Value, AUDIO_EXTS) {
					bmsDir.Logs.addNewLog(Error, noFileMessage(bmsFile.Path, def.command(), def.Value))
				}
			}
		}
		for _, def := range bmsFile.HeaderBmp {
			if def.Value != "" {
				exts := IMAGE_EXTS
				if hasExts(def.Value, MOVIE_EXTS) {
					exts = MOVIE_EXTS
				}
				if !containsInNonBmsFiles(def.Value, exts) {
					bmsDir.Logs.addNewLog(Error, noFileMessage(bmsFile.Path, def.command(), def.Value))
				}
			}
		}
	}

	// check ununified definitions
	unifyCommands := []string{"stagefile", "banner", "backbmp", "preview"}
	isNotUnified := make([]bool, len(unifyCommands))
	values := make([][]string, len(unifyCommands))
	for i, bmsFile := range bmsDir.BmsFiles {
		for j, uc := range unifyCommands {
			values[j] = append(values[j], bmsFile.Header[uc])
			if i > 0 && values[j][i-1] != bmsFile.Header[uc] {
				isNotUnified[j] = true
			}
		}
	}
	for index, uc := range unifyCommands {
		if isNotUnified[index] {
			strs := []string{}
			for j, bmsFile := range bmsDir.BmsFiles {
				strs = append(strs, fmt.Sprintf("%s: %s", relativePathFromBmsRoot(bmsFile.Path), values[index][j]))
			}
			log := Log{Level: Warning, Message: fmt.Sprintf("#%s is not unified", strings.ToUpper(uc)), SubLogs: strs}
			bmsDir.Logs = append(bmsDir.Logs, log)
		}
	}

	// check unused files and empty directories
	isPreview := func(path string) bool {
		for _, ext := range AUDIO_EXTS {
			if regexp.MustCompile(`^preview.*` + ext + `$`).MatchString(relativePathFromBmsRoot(path)) {
				return true
			}
		}
		return false
	}
	ignoreExts := []string{".txt", ".zip", ".rar", ".lzh", ".7z"}
	for _, nonBmsFile := range bmsDir.NonBmsFiles {
		if !nonBmsFile.Used && !hasExts(nonBmsFile.Path, ignoreExts) && !isPreview(nonBmsFile.Path) {
			bmsDir.Logs.addNewLog(Notice, "This file is not used: "+relativePathFromBmsRoot(nonBmsFile.Path))
		}
	}
	for _, dir := range bmsDir.Directories {
		if len(dir.BmsFiles) == 0 && len(dir.NonBmsFiles) == 0 && len(dir.Directories) == 0 {
			bmsDir.Logs.addNewLog(Notice, "This directory is empty: "+relativePathFromBmsRoot(dir.Path))
		}
	}

	// check environment-dependent filename (must do after used check)
	filenameLog := func(path string) {
		bmsDir.Logs.addNewLog(Warning, "This filename has environment-dependent characters: "+path)
	}
	for _, file := range bmsDir.BmsFiles {
		if rPath := relativePathFromBmsRoot(file.Path); containsMultibyteRune(rPath) {
			filenameLog(rPath)
		}
	}
	for _, file := range bmsDir.NonBmsFiles {
		if rPath := relativePathFromBmsRoot(file.Path); (file.Used || strings.ToLower(filepath.Ext(file.Path)) == ".txt" || isPreview(file.Path)) &&
			containsMultibyteRune(rPath) {
			filenameLog(rPath)
		}
	}

	// check over 1 minute audio file (must do after used check)
	for _, file := range bmsDir.NonBmsFiles {
		if file.Used && hasExts(file.Path, AUDIO_EXTS) {
			if d, _ := audio.Duration(file.Path); d >= 60.0 {
				bmsDir.Logs.addNewLog(Warning, fmt.Sprintf("This audio file is over 1 minute(%.1fsec): %s", d, relativePathFromBmsRoot(file.Path)))
			}
		}
	}

	// check pairs of bmsfiles with same hash
	for i := 0; i < len(bmsDir.BmsFiles); i++ {
		for j := i + 1; j < len(bmsDir.BmsFiles); j++ {
			if bmsDir.BmsFiles[i].Sha256 == bmsDir.BmsFiles[j].Sha256 {
				bmsDir.Logs.addNewLog(Warning, fmt.Sprintf("These bmsfiles are same: %s %s", bmsDir.BmsFiles[i].Path, bmsDir.BmsFiles[j].Path))
			}
		}
	}

	// diff
	// TODO ファイルごとの比較ではなく、WAB/BMP定義・WAB/BMP配置でまとめて比較する？
	if doDiffCheck {
		missingLog := func(path, val string) string {
			return fmt.Sprintf("Missing(%s): %s", relativePathFromBmsRoot(path), val)
		}
		for i := 0; i < len(bmsDir.BmsFiles); i++ {
			for j := i + 1; j < len(bmsDir.BmsFiles); j++ {
				diffDefs := func(t objType, iBmsFile, jBmsFile *BmsFile) {
					iDefs, jDefs := iBmsFile.headerIndexedDefs(t), jBmsFile.headerIndexedDefs(t)
					iDefStrs, jDefStrs := []string{}, []string{}
					for _, def := range iDefs {
						iDefStrs = append(iDefStrs, fmt.Sprintf("#%s %s", strings.ToUpper(def.CommandName), def.Value))
					}
					for _, def := range jDefs {
						jDefStrs = append(jDefStrs, fmt.Sprintf("#%s %s", strings.ToUpper(def.CommandName), def.Value))
					}
					ed, ses := diff.Onp(iDefStrs, jDefStrs)
					if ed > 0 {
						log := newLog(Warning, fmt.Sprintf("There are %d differences in %s definitions: %s %s",
							ed, t.string(), relativePathFromBmsRoot(iBmsFile.Path), relativePathFromBmsRoot(jBmsFile.Path)))
						ii, jj := 0, 0
						for _, r := range ses {
							switch r {
							case '=':
								ii++
								jj++
							case '+':
								log.SubLogs = append(log.SubLogs, missingLog(iBmsFile.Path, jDefStrs[jj]))
								jj++
							case '-':
								log.SubLogs = append(log.SubLogs, missingLog(jBmsFile.Path, iDefStrs[ii]))
								ii++
							}
						}
						bmsDir.Logs = append(bmsDir.Logs, *log)
					}
				}
				diffDefs(Wav, &bmsDir.BmsFiles[i], &bmsDir.BmsFiles[j])
				diffDefs(Bmp, &bmsDir.BmsFiles[i], &bmsDir.BmsFiles[j])

				diffObjs := func(t objType, iBmsFile, jBmsFile *BmsFile) {
					logs := []string{}
					iObjs, jObjs := iBmsFile.bmsObjs(t), jBmsFile.bmsObjs(t)
					ii, jj := 0, 0
					for ii < len(iObjs) && jj < len(jObjs) {
						iObj, jObj := iObjs[ii], jObjs[jj]
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
							if iBmsFile.definedValue(t, iObj.value36()) != "" {
								logs = append(logs, missingLog(jBmsFile.Path, iObj.string(iBmsFile)))
							}
							ii++
						} else {
							if jBmsFile.definedValue(t, jObj.value36()) != "" {
								logs = append(logs, missingLog(iBmsFile.Path, jObj.string(jBmsFile)))
							}
							jj++
						}
					}
					for ; ii < len(iObjs); ii++ {
						iObj := iObjs[ii]
						if !iObj.IsLNEnd && iBmsFile.definedValue(t, iObj.value36()) != "" {
							logs = append(logs, missingLog(jBmsFile.Path, iObj.string(iBmsFile)))
						}
					}
					for ; jj < len(jObjs); jj++ {
						jObj := jObjs[jj]
						if !jObj.IsLNEnd && jBmsFile.definedValue(t, jObj.value36()) != "" {
							logs = append(logs, missingLog(iBmsFile.Path, jObj.string(jBmsFile)))
						}
					}
					if len(logs) > 0 {
						log := Log{Level: Warning, Message: fmt.Sprintf("There are %d differences in %s objects: %s, %s",
							len(logs), t.string(), relativePathFromBmsRoot(iBmsFile.Path), relativePathFromBmsRoot(jBmsFile.Path)), SubLogs: logs}
						bmsDir.Logs = append(bmsDir.Logs, log)
					}
				}
				diffObjs(Wav, &bmsDir.BmsFiles[i], &bmsDir.BmsFiles[j])
				diffObjs(Bmp, &bmsDir.BmsFiles[i], &bmsDir.BmsFiles[j])
			}
		}
	}
}

func hasExts(path string, exts []string) bool {
	for _, ext := range exts {
		if strings.EqualFold(ext, filepath.Ext(path)) {
			return true
		}
	}
	return false
}

func IsBmsFile(path string) bool {
	bmsExts := []string{".bms", ".bme", ".bml", ".pms", ".bmson"}
	return hasExts(path, bmsExts)
}

func IsBmsDirectory(path string) bool {
	files, err := ioutil.ReadDir(path)
	if err != nil {
		return false
	}
	for _, f := range files {
		if IsBmsFile(f.Name()) {
			return true
		}
	}
	return false
}

func containsMultibyteRune(text string) bool {
	return len(text) != utf8.RuneCountInString(text)
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
