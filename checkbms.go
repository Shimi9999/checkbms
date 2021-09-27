package checkbms

import (
	"crypto/sha256"
	"fmt"
	"io"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Shimi9999/checkbms/bmson"
	"github.com/saintfish/chardet"
)

type File struct {
	Path string
}

type Directory struct {
	File
	BmsFiles   []BmsFile
	BmsonFiles []BmsonFile
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
	return d.LogStringWithLang(base, "en")
}
func (d Directory) LogStringWithLang(base bool, lang string) string {
	str := ""
	if len(d.Logs) > 0 {
		dirPath := filepath.Clean(d.Path)
		if dirPath == "." {
			dirPath, _ = filepath.Abs(dirPath)
			dirPath = filepath.Base(dirPath)
		} else if base {
			dirPath = filepath.Base(d.Path)
		}
		if lang == "ja" {
			str += fmt.Sprintf("## BMSディレクトリ チェックログ: %s\n", dirPath)
		} else {
			str += fmt.Sprintf("## BmsDirectory checklog: %s\n", dirPath)
		}
		str += d.Logs.StringWithLang(lang)
	}
	return str
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

type BmsFileBase struct {
	File
	FullText   []byte
	Sha256     string
	Keymode    int // 5, 7, 9, 10, 14, 24, 48
	TotalNotes int
	Logs       Logs
}

type Bms struct {
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
}

type BmsFile struct {
	BmsFileBase
	Bms
}

func NewBmsFile(bmsFileBase *BmsFileBase) *BmsFile {
	var bf BmsFile
	bf.BmsFileBase = *bmsFileBase
	bf.Header = make(map[string]string)
	return &bf
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
		return bf.BmsBmpObjs
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
		if bf.BmsWavObjs[i].value36() == bf.lnobj() {
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
func (bf BmsFile) lnobj() string {
	return strings.ToLower(bf.Header["lnobj"])
}
func (bf BmsFile) LogString(base bool) string {
	return bf.LogStringWithLang(base, "en")
}
func (bf BmsFile) LogStringWithLang(base bool, lang string) string {
	str := ""
	if len(bf.Logs) > 0 {
		path := bf.Path
		if base {
			path = filepath.Base(bf.Path)
		}
		if lang == "ja" {
			str += fmt.Sprintf("# BMSファイル チェックログ: %s\n", path)
		} else {
			str += fmt.Sprintf("# BmsFile checklog: %s\n", path)
		}
		str += bf.Logs.StringWithLang(lang)
	}
	return str
}

type BmsonFile struct {
	BmsFileBase
	bmson.Bmson
	IsInvalid bool // bmsonフォーマットエラーで無効なファイル
}

func NewBmsonFile(bmsFileBase *BmsFileBase) *BmsonFile {
	var bf BmsonFile
	bf.BmsFileBase = *bmsFileBase
	return &bf
}
func (bf BmsFileBase) LogString(base bool) string {
	return bf.LogStringWithLang(base, "en")
}
func (bf BmsFileBase) LogStringWithLang(base bool, lang string) string {
	str := ""
	if len(bf.Logs) > 0 {
		path := bf.Path
		if base {
			path = filepath.Base(bf.Path)
		}
		if lang == "ja" {
			str += fmt.Sprintf("# BMSONファイル チェックログ: %s\n", path)
		} else {
			str += fmt.Sprintf("# BmsonFile checklog: %s\n", path)
		}
		str += bf.Logs.StringWithLang(lang)
	}
	return str
}

func CalculateDefaultTotal(totalNotes, keymode int) float64 {
	tn := float64(totalNotes)
	if keymode >= 24 {
		return math.Max(300.0, 7.605*(tn+100.0)/(0.01*tn+6.5))
	} else {
		return math.Max(260.0, 7.605*tn/(0.01*tn+6.5))
	}
}

type NonBmsFile struct {
	File
	Used_bms   bool
	Used_bmson bool
}

func newNonBmsFile(path string) *NonBmsFile {
	var nbf NonBmsFile
	nbf.Path = path
	return &nbf
}

func (f NonBmsFile) UsedFromAny() bool {
	return f.Used_bms || f.Used_bmson
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
		bo.Measure, strings.ToUpper(bo.Channel), bo.Position.Numerator, bo.Position.Denominator, bo.ObjType.string(), strings.ToUpper(val), definedValue)
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
	//Debug   = AlertLevel("DEBUG ERROR")
)

func (al AlertLevel) String_ja() string {
	switch al {
	case Error:
		return "エラー"
	case Warning:
		return "警告"
	case Notice:
		return "通知"
		/*case Debug:
		return "デバッグ"*/
	}
	return ""
}

type SubLogType int

const (
	List SubLogType = iota
	Detail
)

type Log struct {
	Level      AlertLevel
	Message    string
	Message_ja string
	SubLogs    []string
	SubLogType SubLogType
}

func (log Log) String() string {
	return log.StringWithLang("en")
}
func (log Log) StringWithLang(lang string) (str string) {
	//lang = "ja"
	level := string(log.Level)
	message := log.Message
	if lang == "ja" && log.Message_ja != "" {
		message = log.Message_ja
		level = log.Level.String_ja()
	}
	if len(log.SubLogs) > 0 {
		// ListならログにSubLogをくっつけて複製、Detailならログの次にSubLogを補足ログとして追加
		if log.SubLogType == List {
			for i, subLog := range log.SubLogs {
				str += level + ": " + message + ": " + subLog
				if i < len(log.SubLogs)-1 {
					str += "\n"
				}
			}
		} else {
			str += level + ": " + message
			for _, subLog := range log.SubLogs {
				str += "\n  " + subLog
			}
		}
	} else {
		str += level + ": " + message
	}
	return str
}

type Logs []Log

/*func (logs *Logs) addLogFromResult(result interface{}) {
	//fmt.Println(reflect.TypeOf(result), reflect.TypeOf(result).Implements(cr), reflect.TypeOf(result).Elem().Implements(cr))
	if r, ok := result.(checkResult); ok {
		if r != nil {
			*logs = append(*logs, r.Log())
		}
	} else if reflect.TypeOf(result).Kind() == reflect.Slice && reflect.TypeOf(result).Elem().Implements(reflect.TypeOf((*checkResult)(nil)).Elem()) {
		results := reflect.ValueOf(result)
		for i := 0; i < results.Len(); i++ {
			r := results.Index(i).Interface().(checkResult)
			if r != nil {
				*logs = append(*logs, r.Log())
			}
		}
	}
}*/
func (logs Logs) String() string {
	return logs.StringWithLang("en")
}
func (logs Logs) StringWithLang(lang string) string {
	var str string
	for i, log := range logs {
		if i > 0 {
			str += "\n"
		}
		str += log.StringWithLang(lang)
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
		if bv, ok := c.BoundaryValue.([]int); ok && len(bv) == 2 {
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
		if bv, ok := c.BoundaryValue.([]float64); ok && len(bv) == 2 {
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
		if bvs, ok := c.BoundaryValue.([]string); ok && len(bvs) >= 1 {
			for _, bv := range bvs {
				if regexp.MustCompile(bv).MatchString(value) {
					return true, nil
				}
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
	{"player", Int, Necessary, []int{1, 4}},
	{"genre", String, Semi_necessary, nil},
	{"title", String, Necessary, nil},
	{"artist", String, Semi_necessary, nil},
	{"subtitle", String, Unnecessary, nil},
	{"subartist", String, Unnecessary, nil},
	{"bpm", Float, Necessary, []float64{math.SmallestNonzeroFloat64, math.MaxFloat64}},
	{"playlevel", Int, Semi_necessary, []int{0, math.MaxInt64}},
	{"rank", Int, Semi_necessary, []int{0, 4}},
	{"defexrank", Float, Unnecessary, []float64{0, math.MaxFloat64}},
	{"total", Float, Semi_necessary, []float64{0, math.MaxFloat64}},
	{"difficulty", Int, Semi_necessary, []int{0, 5}},
	{"stagefile", Path, Unnecessary, IMAGE_EXTS},
	{"banner", Path, Unnecessary, IMAGE_EXTS},
	{"backbmp", Path, Unnecessary, IMAGE_EXTS},
	{"preview", Path, Unnecessary, AUDIO_EXTS},
	{"lntype", Int, Unnecessary, []int{1, 2}},
	{"lnobj", String, Unnecessary, []string{`^[0-9a-zA-Z]{2}$`}},
	{"lnmode", Int, Unnecessary, []int{1, 3}},
	{"volwav", Int, Unnecessary, []int{0, math.MaxInt64}},
	{"comment", String, Unnecessary, nil},
}

var INDEXED_COMMANDS = []Command{
	{"wav", Path, Necessary, AUDIO_EXTS},
	{"bmp", Path, Unnecessary, BMP_EXTS},
	{"bpm", Float, Unnecessary, []float64{math.SmallestNonzeroFloat64, math.MaxFloat64}},
	{"stop", Float, Unnecessary, []float64{math.SmallestNonzeroFloat64, math.MaxFloat64}},
	{"scroll", Float, Unnecessary, []float64{-math.MaxFloat64, math.MaxFloat64}},
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
		bmsDir, err := ScanBmsDirectory(path, true, true)
		if err != nil {
			return nil, err
		}
		bmsDirs = append(bmsDirs, *bmsDir)
	} else {
		files, err := os.ReadDir(path)
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

func ScanBmsDirectory(path string, isRootDir, doScan bool) (*Directory, error) {
	dir := newDirectory(path)
	files, _ := os.ReadDir(path)

	for _, f := range files {
		filePath := filepath.Join(path, f.Name())
		if isRootDir && IsBmsFile(f.Name()) { // TODO isRootDirいる？on/off付ける？
			bmsFileBase, err := ReadBmsFileBase(filePath)
			if err != nil {
				return nil, err
			}
			if IsBmsonFile(filePath) {
				bmsonFile := NewBmsonFile(bmsFileBase)
				if doScan {
					err := bmsonFile.ScanBmsonFile()
					if err != nil {
						return nil, err
					}
				}
				dir.BmsonFiles = append(dir.BmsonFiles, *bmsonFile)
			} else {
				bmsFile := NewBmsFile(bmsFileBase)
				if doScan {
					err := bmsFile.ScanBmsFile()
					if err != nil {
						return nil, err
					}
				}
				dir.BmsFiles = append(dir.BmsFiles, *bmsFile)
			}
		} else if f.IsDir() {
			innnerDir, err := ScanBmsDirectory(filePath, false, doScan)
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

func ReadBmsFileBase(path string) (bmsFileBase *BmsFileBase, _ error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("BMSfile open error: " + err.Error())
	}
	defer file.Close()

	bmsFileBase = &BmsFileBase{}
	bmsFileBase.Path = path

	fullText, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("BMSfile ReadAll error: " + err.Error())
	}
	bmsFileBase.FullText = fullText
	bmsFileBase.Sha256 = fmt.Sprintf("%x", sha256.Sum256(bmsFileBase.FullText))

	return bmsFileBase, nil
}

func CheckBmsFile(bmsFile *BmsFile) {
	if logs := CheckHeaderCommands(bmsFile); len(logs) > 0 {
		bmsFile.Logs = append(bmsFile.Logs, logs...)
	}

	if result := CheckTitleAndSubtitleHaveSameText(bmsFile); result != nil {
		bmsFile.Logs = append(bmsFile.Logs, result.Log())
	}

	func() {
		results1, results2, results3, result4 := CheckIndexedDefinitionsHaveInvalidValue(bmsFile)
		for _, result := range results1 {
			bmsFile.Logs = append(bmsFile.Logs, result.Log())
		}
		for _, result := range results2 {
			bmsFile.Logs = append(bmsFile.Logs, result.Log())
		}
		for _, result := range results3 {
			bmsFile.Logs = append(bmsFile.Logs, result.Log())
		}
		if result4 != nil {
			bmsFile.Logs = append(bmsFile.Logs, result4.Log())
		}
	}()

	if result := CheckTotalnotesIsZero(&bmsFile.BmsFileBase); result != nil {
		bmsFile.Logs = append(bmsFile.Logs, result.Log())
	}

	if result := CheckWavObjExistsIn0thMeasure(bmsFile); result != nil {
		bmsFile.Logs = append(bmsFile.Logs, result.Log())
	}

	func() {
		results1, results2 := CheckPlacedObjIsDefinedAndDefinedHeaderIsPlaced(bmsFile)
		for _, result := range results1 {
			bmsFile.Logs = append(bmsFile.Logs, result.Log())
		}
		for _, result := range results2 {
			bmsFile.Logs = append(bmsFile.Logs, result.Log())
		}
	}()

	if result := CheckSoundOfMineExplosionIsUsed(bmsFile); result != nil {
		bmsFile.Logs = append(bmsFile.Logs, result.Log())
	}

	for _, result := range CheckWavDuplicate(bmsFile) {
		bmsFile.Logs = append(bmsFile.Logs, result.Log())
	}

	for _, result := range CheckNoteOverlap(bmsFile) {
		bmsFile.Logs = append(bmsFile.Logs, result.Log())
	}

	func() {
		results1, results2 := CheckEndOfLNExistsAndNotesInLN(bmsFile)
		for _, result := range results1 {
			bmsFile.Logs = append(bmsFile.Logs, result.Log())
		}
		for _, result := range results2 {
			bmsFile.Logs = append(bmsFile.Logs, result.Log())
		}
	}()

	for _, result := range CheckBpmValue(bmsFile) {
		bmsFile.Logs = append(bmsFile.Logs, result.Log())
	}

	func() {
		results1, results2 := CheckMeasureLength(bmsFile)
		for _, result := range results1 {
			bmsFile.Logs = append(bmsFile.Logs, result.Log())
		}
		for _, result := range results2 {
			bmsFile.Logs = append(bmsFile.Logs, result.Log())
		}
	}()

	func() {
		result1, result2 := CheckWithoutKeysound(bmsFile, nil)
		if result1 != nil {
			bmsFile.Logs = append(bmsFile.Logs, result1.Log())
		}
		if result2 != nil {
			bmsFile.Logs = append(bmsFile.Logs, result2.Log())
		}
	}()
}

func CheckBmsDirectory(bmsDir *Directory, doDiffCheck bool) {
	for i := range bmsDir.BmsFiles {
		CheckBmsFile(&bmsDir.BmsFiles[i])

		for _, result := range CheckDefinedFilesExist(bmsDir, &bmsDir.BmsFiles[i]) {
			bmsDir.Logs = append(bmsDir.Logs, result.Log())
		}

		pathsOfdoNotExistWavs := []string{}
		for _, result := range CheckDefinedWavFilesExist(bmsDir, &bmsDir.BmsFiles[i]) {
			bmsDir.Logs = append(bmsDir.Logs, result.Log())
			pathsOfdoNotExistWavs = append(pathsOfdoNotExistWavs, result.filePath)
		}

		for _, result := range CheckDefinedBmpFilesExist(bmsDir, &bmsDir.BmsFiles[i]) {
			bmsDir.Logs = append(bmsDir.Logs, result.Log())
		}

		// count moments and notes without keysound (or audio file)
		if len(pathsOfdoNotExistWavs) > 0 {
			wavFileIsExist := func(path string) bool {
				for _, pathOfdoNotExistWav := range pathsOfdoNotExistWavs {
					if path == pathOfdoNotExistWav {
						return false
					}
				}
				return true
			}
			func() {
				result1, result2 := CheckWithoutKeysound(&bmsDir.BmsFiles[i], wavFileIsExist)
				if result1 != nil {
					bmsDir.BmsFiles[i].Logs = append(bmsDir.BmsFiles[i].Logs, result1.Log())
				}
				if result2 != nil {
					bmsDir.BmsFiles[i].Logs = append(bmsDir.BmsFiles[i].Logs, result2.Log())
				}
			}()
		}
	}

	for i := range bmsDir.BmsonFiles {
		if bmsDir.BmsonFiles[i].IsInvalid {
			continue
		}

		CheckBmsonFile(&bmsDir.BmsonFiles[i])

		for _, result := range CheckDefinedInfoFilesExistBmson(bmsDir, &bmsDir.BmsonFiles[i]) {
			bmsDir.Logs = append(bmsDir.Logs, result.Log())
		}

		pathsOfdoNotExistWavs := []string{}
		for _, result := range CheckDefinedSoundFilesExistBmson(bmsDir, &bmsDir.BmsonFiles[i]) {
			bmsDir.Logs = append(bmsDir.Logs, result.Log())
			pathsOfdoNotExistWavs = append(pathsOfdoNotExistWavs, result.filePath)
		}

		for _, result := range CheckDefinedBgaFilesExistBmson(bmsDir, &bmsDir.BmsonFiles[i]) {
			bmsDir.Logs = append(bmsDir.Logs, result.Log())
		}

		if len(pathsOfdoNotExistWavs) > 0 {
			wavFileIsExist := func(path string) bool {
				for _, pathOfdoNotExistWav := range pathsOfdoNotExistWavs {
					if path == pathOfdoNotExistWav {
						return false
					}
				}
				return true
			}
			func() {
				result1, result2 := CheckWithoutKeysoundBmson(&bmsDir.BmsonFiles[i], wavFileIsExist)
				if result1 != nil {
					bmsDir.BmsonFiles[i].Logs = append(bmsDir.BmsonFiles[i].Logs, result1.Log())
				}
				if result2 != nil {
					bmsDir.BmsonFiles[i].Logs = append(bmsDir.BmsonFiles[i].Logs, result2.Log())
				}
			}()
		}
	}

	for _, result := range CheckDefinitionsAreUnified(bmsDir) {
		bmsDir.Logs = append(bmsDir.Logs, result.Log())
	}

	for _, result := range CheckUnusedFile(bmsDir) {
		bmsDir.Logs = append(bmsDir.Logs, result.Log())
	}

	for _, result := range CheckEmptyDirectory(bmsDir) {
		bmsDir.Logs = append(bmsDir.Logs, result.Log())
	}

	for _, result := range CheckEnvironmentDependentFilename(bmsDir) {
		bmsDir.Logs = append(bmsDir.Logs, result.Log())
	}

	for _, result := range CheckOver1MinuteAudioFile(bmsDir) {
		bmsDir.Logs = append(bmsDir.Logs, result.Log())
	}

	for _, result := range CheckSameHashBmsFiles(bmsDir) {
		bmsDir.Logs = append(bmsDir.Logs, result.Log())
	}

	for _, result := range CheckIndexedDefinitionAreUnified(bmsDir) {
		bmsDir.Logs = append(bmsDir.Logs, result.Log())
	}

	for _, result := range CheckObjectStructreIsUnified(bmsDir) {
		bmsDir.Logs = append(bmsDir.Logs, result.Log())
	}

	// diff
	// TODO ファイルごとの比較ではなく、WAB/BMP定義・WAB/BMP配置でまとめて比較する？
	if doDiffCheck {
		for i := 0; i < len(bmsDir.BmsFiles); i++ {
			for j := i + 1; j < len(bmsDir.BmsFiles); j++ {
				for _, result := range CheckDefinitionDiff(bmsDir.Path, &bmsDir.BmsFiles[i], &bmsDir.BmsFiles[j]) {
					bmsDir.Logs = append(bmsDir.Logs, result.Log())
				}
				for _, result := range CheckObjectDiff(bmsDir.Path, &bmsDir.BmsFiles[i], &bmsDir.BmsFiles[j]) {
					bmsDir.Logs = append(bmsDir.Logs, result.Log())
				}
			}
		}
	}
}

func hasExts(path string, exts []string) bool {
	pathExt := filepath.Ext(path)
	for _, ext := range exts {
		if strings.EqualFold(ext, pathExt) {
			return true
		}
	}
	return false
}

func IsBmsFile(path string) bool {
	bmsExts := []string{".bms", ".bme", ".bml", ".pms", ".bmson"}
	return hasExts(path, bmsExts)
}

func IsBmsonFile(path string) bool {
	bmsExts := []string{".bmson"}
	return hasExts(path, bmsExts)
}

func IsBmsDirectory(path string) bool {
	files, err := os.ReadDir(path)
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

func isUTF8(fBytes []byte) (bool, error) {
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
