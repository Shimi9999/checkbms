package checkbms

// TODO 全てのCheckでbmsFileのLogsにresultのLogsを追加してからreturnすべき？

import (
	"bufio"
	"bytes"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/transform"
)

type checkResult interface {
	Log() Log
}

type duplicateDefinition struct {
	command  string
	oldValue string
	newValue string
}

func (dd duplicateDefinition) Log() Log {
	return Log{
		Level:      Warning,
		Message:    fmt.Sprintf("#%s is duplicate: old= %s, new= %s", strings.ToUpper(dd.command), dd.oldValue, dd.newValue),
		Message_ja: fmt.Sprintf("#%sが重複しています: old= %s, new= %s", strings.ToUpper(dd.command), dd.oldValue, dd.newValue),
	}
}

type invalidLine struct {
	lineNumber int
	line       string
}

func (il invalidLine) Log() Log {
	return Log{
		Level:      Error,
		Message:    fmt.Sprintf("Invalid line(%d): %s", il.lineNumber, il.line),
		Message_ja: fmt.Sprintf("この行は無効です(%d): %s", il.lineNumber, il.line),
	}
}

type bmsFileCharsetIsUtf8 struct {
	hasMultibyteRune bool
}

func (bu bmsFileCharsetIsUtf8) Log() Log {
	if bu.hasMultibyteRune {
		return Log{
			Level:      Error,
			Message:    "Bmsfile charset is UTF-8, not Shift-JIS, and contains multibyte characters",
			Message_ja: "BMSファイルの文字コードがShift-JISではなくUTF-8です。またマルチバイト文字を含んでいます",
		}
	} else {
		return Log{
			Level:      Notice,
			Message:    "Bmsfile charset is UTF-8, not Shift-JIS",
			Message_ja: "BMSファイルの文字コードがShift-JISではなくUTF-8です",
		}
	}
}

func (bmsFile *BmsFile) ScanBmsFile() error {
	if bmsFile.FullText == nil {
		return fmt.Errorf("FullText is empty: %s", bmsFile.Path)
	}

	var dds []duplicateDefinition
	var ils []invalidLine
	var bu *bmsFileCharsetIsUtf8

	const (
		initialBufSize = 10000
		maxBufSize     = 1000000
	)
	scanner := bufio.NewScanner(bytes.NewReader(bmsFile.FullText))
	buf := make([]byte, initialBufSize)
	scanner.Buffer(buf, maxBufSize)

	hasUtf8Bom := false
	hasMultibyteRune := false
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
			return fmt.Errorf("Shift-JIS decode error: " + err.Error())
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
						dds = append(dds, duplicateDefinition{command: command.Name, oldValue: val, newValue: data})
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
								dds = append(dds, duplicateDefinition{command: lineCommand, oldValue: (*defs)[i].Value, newValue: data})
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

		ils = append(ils, invalidLine{lineNumber: lineNumber, line: line})

	correctLine:
	}
	if scanner.Err() != nil {
		return fmt.Errorf("BMSfile scan error: " + scanner.Err().Error())
	}

	bmsFile.sortBmsObjs()
	bmsFile.setIsLNEnd()

	isUtf8 := hasUtf8Bom
	if !isUtf8 {
		var err error
		isUtf8, err = isUTF8(bmsFile.FullText)
		if err != nil {
			isUtf8 = false
		}
	}
	if isUtf8 {
		bu = &bmsFileCharsetIsUtf8{hasMultibyteRune: hasMultibyteRune}
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
			if (chint >= 51 && chint <= 59) || (chint >= 61 && chint <= 69) || obj.value36() == bmsFile.lnobj() {
				lnCount++
			} else {
				bmsFile.TotalNotes++
			}
		}
	}
	lnmode, _ := strconv.Atoi(bmsFile.Header["lnmode"])
	if lnmode >= 2 {
		bmsFile.TotalNotes += lnCount
	} else {
		bmsFile.TotalNotes += lnCount / 2
	}

	if filepath.Ext(bmsFile.Path) == ".pms" {
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

	for _, result := range dds {
		bmsFile.Logs = append(bmsFile.Logs, result.Log())
	}
	for _, result := range ils {
		bmsFile.Logs = append(bmsFile.Logs, result.Log())
	}
	if bu != nil {
		bmsFile.Logs = append(bmsFile.Logs, bu.Log())
	}
	/*bmsFile.Logs.addLogFromResult(dds)
	bmsFile.Logs.addLogFromResult(ils)
	bmsFile.Logs.addLogFromResult(bu)*/

	return nil
}

/*type missingDefinition struct {
	command Command
}

func (md missingDefinition) Log() Log {
	alertLevel := Error
	if md.command.Necessity == Semi_necessary {
		alertLevel = Warning
	}
	return Log{
		Level:      alertLevel,
		Message:    fmt.Sprintf("#%s definition is missing", strings.ToUpper(md.command.Name)),
		Message_ja: fmt.Sprintf("#%sxx定義が見つかりません", strings.ToUpper(md.command.Name)),
	}
}*/

func CheckHeaderCommands(bmsFile *BmsFile) (logs Logs) {
	for _, command := range COMMANDS {
		val, ok := bmsFile.Header[command.Name]
		if !ok {
			if command.Necessity != Unnecessary {
				alertLevel := Error
				if command.Necessity == Semi_necessary {
					alertLevel = Warning
				}
				logs = append(logs, Log{
					Level:      alertLevel,
					Message:    fmt.Sprintf("#%s definition is missing", strings.ToUpper(command.Name)),
					Message_ja: fmt.Sprintf("#%s定義が見つかりません", strings.ToUpper(command.Name)),
				})
			}
		} else if val == "" {
			logs = append(logs, Log{
				Level:      Warning,
				Message:    fmt.Sprintf("#%s value is empty", strings.ToUpper(command.Name)),
				Message_ja: fmt.Sprintf("#%sの値が空です", strings.ToUpper(command.Name)),
			})
		} else if isInRange, err := command.isInRange(val); err != nil || !isInRange {
			/*if err != nil {
				fmt.Printf("DEBUG ERROR: isInRange return error(%s): command= %s, value= %s\n", err.Error(), command.Name, val)
			}*/
			logs = append(logs, Log{
				Level:      Error,
				Message:    fmt.Sprintf("#%s has invalid value: %s", strings.ToUpper(command.Name), val),
				Message_ja: fmt.Sprintf("#%sが無効な値です: %s", strings.ToUpper(command.Name), val),
			})
		} else if command.Name == "rank" { // TODO ここらへんはCommand型のCheck関数的なものに置き換えたい？
			rank, _ := strconv.Atoi(val)
			rankText := ""
			if rank == 0 {
				rankText = "0(VERY HARD)"
			} else if rank == 1 {
				rankText = "1(HARD)"
			} else if rank == 4 {
				rankText = "4(VERY EASY)"
			}
			if rankText != "" {
				logs = append(logs, Log{
					Level:      Notice,
					Message:    fmt.Sprintf("#RANK is %s", rankText),
					Message_ja: fmt.Sprintf("#RANKが%sです", rankText),
				})
			}
		} else if command.Name == "total" {
			total, _ := strconv.ParseFloat(val, 64)
			if total < 100 {
				logs = append(logs, Log{
					Level:      Warning,
					Message:    fmt.Sprintf("#TOTAL is under 100: %s", val),
					Message_ja: fmt.Sprintf("#TOTALが100未満です: %s", val),
				})
			} else if bmsFile.TotalNotes > 0 {
				defaultTotal := bmsFile.calculateDefaultTotal()
				overRate := 1.6
				totalPerNotes := total / float64(bmsFile.TotalNotes) // TODO 適切な基準値は？
				if total > defaultTotal*overRate && totalPerNotes > 0.35 {
					logs = append(logs, Log{
						Level:      Notice,
						Message:    fmt.Sprintf("#TOTAL is very high(TotalNotes=%d): %s", bmsFile.TotalNotes, val),
						Message_ja: fmt.Sprintf("#TOTALがかなり高いです(トータルノーツ=%d): %s", bmsFile.TotalNotes, val),
					})
				} else if total < defaultTotal/overRate && totalPerNotes < 0.2 {
					logs = append(logs, Log{
						Level:      Notice,
						Message:    fmt.Sprintf("#TOTAL is very low(TotalNotes=%d): %s", bmsFile.TotalNotes, val),
						Message_ja: fmt.Sprintf("#TOTALがかなり低いです(トータルノーツ=%d): %s", bmsFile.TotalNotes, val),
					})
				}
			}
		} else if command.Name == "difficulty" {
			if val == "0" {
				logs = append(logs, Log{
					Level:      Warning,
					Message:    "#DIFFICULTY is 0(Undefined)",
					Message_ja: "#DIFFICULTYが0(未定義)です",
				})
			}
		} else if command.Name == "defexrank" {
			logs = append(logs, Log{
				Level:      Notice,
				Message:    "#DEFEXRANK is defined: " + val,
				Message_ja: "#DEFEXRANKが定義されています: " + val,
			})
		} else if command.Name == "lntype" {
			if val == "2" {
				logs = append(logs, Log{
					Level:      Warning,
					Message:    "#LNTYPE 2(MGQ) is deprecated",
					Message_ja: "#LNTYPE 2(MGQ)は非推奨です",
				})
			}
		}
	}
	return logs
}

type titleAndSubtitleHaveSameText struct {
	subtitle string
}

func (t titleAndSubtitleHaveSameText) Log() Log {
	return Log{
		Level:      Warning,
		Message:    "The end of #TITLE contains the same string as #SUBTITLE: " + t.subtitle,
		Message_ja: "#TITLEの末尾に#SUBTITLEと同じ文字列を含んでいます:" + t.subtitle,
	}
}

func CheckTitleAndSubtitleHaveSameText(bmsFile *BmsFile) (ts *titleAndSubtitleHaveSameText) {
	title, ok1 := bmsFile.Header["title"]
	subtitle, ok2 := bmsFile.Header["subtitle"]
	// TODO 括弧を取り外す＆括弧を付ける？
	if ok1 && ok2 && subtitle != "" && strings.HasSuffix(strings.ToLower(title), strings.ToLower(subtitle)) {
		ts = &titleAndSubtitleHaveSameText{subtitle: subtitle}
	}
	return ts
}

type missingIndexedDefinition struct {
	command Command
}

func (md missingIndexedDefinition) Log() Log {
	alertLevel := Error
	if md.command.Necessity == Semi_necessary {
		alertLevel = Warning
	}
	return Log{
		Level:      alertLevel,
		Message:    fmt.Sprintf("#%sxx definition is missing", strings.ToUpper(md.command.Name)),
		Message_ja: fmt.Sprintf("#%sxxの定義が見つかりません", strings.ToUpper(md.command.Name)),
	}
}

type emptyDefinition struct {
	definition indexedDefinition
}

func (ed emptyDefinition) Log() Log {
	return Log{
		Level:      Warning,
		Message:    fmt.Sprintf("#%s value is empty", strings.ToUpper(ed.definition.command())),
		Message_ja: fmt.Sprintf("#%sの値が空です", strings.ToUpper(ed.definition.command())),
	}
}

type invalidValueOfIndexedCommand struct {
	definition indexedDefinition
}

func (iv invalidValueOfIndexedCommand) Log() Log {
	return Log{
		Level:      Error,
		Message:    fmt.Sprintf("#%s has invalid value: %s", strings.ToUpper(iv.definition.command()), iv.definition.Value),
		Message_ja: fmt.Sprintf("#%sが無効な値です: %s", strings.ToUpper(iv.definition.command()), iv.definition.Value),
	}
}

type noWavExtDefs struct {
	noWavExtDefs []indexedDefinition
}

func (nwd noWavExtDefs) Log() Log {
	return Log{
		Level: Notice,
		Message: fmt.Sprintf("#WAV definition has non-.wav extension(*%d): %s %s etc...",
			len(nwd.noWavExtDefs), strings.ToUpper(nwd.noWavExtDefs[0].command()), nwd.noWavExtDefs[0].Value),
		Message_ja: fmt.Sprintf("#WAVに拡張子.wavでない定義があります(*%d): %s %s etc...",
			len(nwd.noWavExtDefs), strings.ToUpper(nwd.noWavExtDefs[0].command()), nwd.noWavExtDefs[0].Value),
	} // TODO SubLogに表示する？必要なさそう
}

func CheckIndexedDefinitionsHaveInvalidValue(bmsFile *BmsFile) (mds []missingIndexedDefinition, eds []emptyDefinition, ivs []invalidValueOfIndexedCommand, nwd *noWavExtDefs) {
	headerIndexedDefinitions := [][]indexedDefinition{bmsFile.HeaderWav, bmsFile.HeaderBmp, bmsFile.HeaderExtendedBpm, bmsFile.HeaderStop, bmsFile.HeaderScroll}
	hasNoWavExtDefs := []indexedDefinition{}
	for i, defs := range headerIndexedDefinitions {
		if len(defs) == 0 {
			if INDEXED_COMMANDS[i].Necessity != Unnecessary {
				mds = append(mds, missingIndexedDefinition{command: INDEXED_COMMANDS[i]})
			}
		}
		for _, def := range defs {
			if def.Value == "" {
				eds = append(eds, emptyDefinition{definition: def})
			} else if isInRange, err := INDEXED_COMMANDS[i].isInRange(def.Value); err != nil || !isInRange {
				/*if err != nil {
					fmt.Printf("DEBUG ERROR: isInRange return error(%s): command= %s, value= %s\n", err.Error(), INDEXED_COMMANDS[i].Name, def.Value)
				}*/
				ivs = append(ivs, invalidValueOfIndexedCommand{definition: def})
			} else if def.CommandName == "wav" && strings.ToLower(filepath.Ext(def.Value)) != ".wav" {
				hasNoWavExtDefs = append(hasNoWavExtDefs, def)
			}
		}
	}
	if len(hasNoWavExtDefs) > 0 {
		nwd = &noWavExtDefs{noWavExtDefs: hasNoWavExtDefs}
	}
	return mds, eds, ivs, nwd
}

type totalnotesIsZero struct{}

func (tz totalnotesIsZero) Log() Log {
	return Log{
		Level:      Error,
		Message:    "TotalNotes is 0",
		Message_ja: "トータルノーツ数が0です",
	}
}

func CheckTotalnotesIsZero(bmsFile *BmsFile) *totalnotesIsZero {
	if bmsFile.TotalNotes == 0 {
		return &totalnotesIsZero{}
	}
	return nil
}

type wavObjsIn0thMeasure struct {
	wavObjs []bmsObj
	bmsFile *BmsFile
}

func (wo wavObjsIn0thMeasure) Log() Log {
	log := Log{
		Level:      Warning,
		Message:    "Note exists in 0th measure",
		Message_ja: "0小節目にノーツが配置されています",
		SubLogs:    []string{}, //[]SubLog{},
		SubLogType: List,
	}
	for _, wavObj := range wo.wavObjs {
		//log.SubLogs = append(log.SubLogs, SubLog{Message: fmt.Sprintf("%s", wavObj.string(wo.bmsFile))})
		log.SubLogs = append(log.SubLogs, fmt.Sprintf("%s", wavObj.string(wo.bmsFile)))
	}
	return log
}

func CheckWavObjExistsIn0thMeasure(bmsFile *BmsFile) *wavObjsIn0thMeasure {
	detectedObjs := []bmsObj{}
	for _, obj := range bmsFile.BmsWavObjs {
		if obj.Measure != 0 {
			break
		}
		if matchChannel(obj.Channel, NOTE_CHANNELS) {
			detectedObjs = append(detectedObjs, obj)
		}
	}
	if len(detectedObjs) > 0 {
		return &wavObjsIn0thMeasure{wavObjs: detectedObjs, bmsFile: bmsFile}
	}
	return nil
}

type placedUndefinedObj struct {
	oType     objType
	objs      []bmsObj
	objValues []string
}

func (puo *placedUndefinedObj) initObjValues() {
	for _, obj := range puo.objs {
		puo.objValues = append(puo.objValues, obj.value36())
	}
	puo.objValues = removeDuplicate(puo.objValues)
	sort.Slice(puo.objValues, func(i, j int) bool {
		ii, _ := strconv.ParseInt(puo.objValues[i], 36, 32)
		ij, _ := strconv.ParseInt(puo.objValues[j], 36, 32)
		return ii < ij
	})
}

func (puo *placedUndefinedObj) Log() Log {
	log := Log{
		Level:      Warning,
		Message:    fmt.Sprintf("Placed %s object is undefined", puo.oType.string()),
		Message_ja: fmt.Sprintf("配置されている%sオブジェが未定義です", puo.oType.string()),
		SubLogs:    []string{}, //[]SubLog{},
		SubLogType: List,
	}
	if puo.objValues == nil {
		puo.initObjValues()
	}
	for _, objValue := range puo.objValues {
		//log.SubLogs = append(log.SubLogs, SubLog{Message: fmt.Sprintf("%s", objValue)})
		log.SubLogs = append(log.SubLogs, strings.ToUpper(objValue))
	}
	return log
}

type definedUnplacedObj struct {
	oType objType
	defs  []indexedDefinition
}

func (duo definedUnplacedObj) Log() Log {
	log := Log{
		Level:      Warning,
		Message:    fmt.Sprintf("Defined %s object is not placed", duo.oType.string()),
		Message_ja: fmt.Sprintf("定義されている%sオブジェが未配置です", duo.oType.string()),
		SubLogs:    []string{}, //[]SubLog{},
		SubLogType: List,
	}
	for _, def := range duo.defs {
		//log.SubLogs = append(log.SubLogs, SubLog{Message: fmt.Sprintf("%s (%s)", strings.ToUpper(def.Index), def.Value)})
		log.SubLogs = append(log.SubLogs, fmt.Sprintf("%s (%s)", strings.ToUpper(def.Index), def.Value))
	}
	return log
}

// この実装だとログの出力される順番が変わる？Placed→Defined→Placed→Defined→...だったのがPlaced*5→Defined*5になる
func CheckPlacedObjIsDefinedAndDefinedHeaderIsPlaced(bmsFile *BmsFile) (puos []placedUndefinedObj, duos []definedUnplacedObj) {
	checkDefinedAndPlaced := func(t objType, definitions []indexedDefinition, objs []bmsObj, ignoreDef string, ignoreObj string) (puo *placedUndefinedObj, duo *definedUnplacedObj) {
		usedObjs := map[string]bool{}
		for _, def := range definitions {
			usedObjs[def.Index] = false
		}
		undefinedObjs := []bmsObj{}
		for _, obj := range objs {
			if _, ok := usedObjs[obj.value36()]; ok {
				usedObjs[obj.value36()] = true
			} else {
				if obj.value36() != ignoreObj {
					undefinedObjs = append(undefinedObjs, obj)
				}
			}
		}
		if len(undefinedObjs) > 0 {
			puo = &placedUndefinedObj{oType: t, objs: undefinedObjs}
		}
		unplacedDefs := []indexedDefinition{}
		for _, def := range definitions {
			if !usedObjs[def.Index] && def.Index != ignoreDef {
				unplacedDefs = append(unplacedDefs, def)
			}
		}
		if len(unplacedDefs) > 0 {
			duo = &definedUnplacedObj{oType: t, defs: unplacedDefs}
		}
		return puo, duo
	}
	puosTmp := [5]*placedUndefinedObj{}
	duosTmp := [5]*definedUnplacedObj{}
	puosTmp[0], duosTmp[0] = checkDefinedAndPlaced(Bmp, bmsFile.HeaderBmp, bmsFile.BmsBmpObjs, "00", "") // 00:misslayer
	puosTmp[1], duosTmp[1] = checkDefinedAndPlaced(Wav, bmsFile.HeaderWav, bmsFile.BmsWavObjs, "00", bmsFile.lnobj())
	puosTmp[2], duosTmp[2] = checkDefinedAndPlaced(Bpm, bmsFile.HeaderExtendedBpm, bmsFile.BmsExtendedBpmObjs, "", "")
	puosTmp[3], duosTmp[3] = checkDefinedAndPlaced(Stop, bmsFile.HeaderStop, bmsFile.BmsStopObjs, "", "")
	puosTmp[4], duosTmp[4] = checkDefinedAndPlaced(Scroll, bmsFile.HeaderScroll, bmsFile.BmsScrollObjs, "", "")
	for _, puo := range puosTmp {
		if puo != nil {
			puos = append(puos, *puo)
		}
	}
	for _, duo := range duosTmp {
		if duo != nil {
			duos = append(duos, *duo)
		}
	}
	//puos = append(puos, *bmPuo, *wavPuo, *bpmPuo, *stpPuo, *scrPuo)
	//duos = append(duos, *bmpDuo, *wavDuo, *bpmDuo, *stpDuo, *scrDuo)
	return puos, duos
}

type unusedMineSound struct {
	value string
}

func (um unusedMineSound) Log() Log {
	return Log{
		Level:      Warning,
		Message:    fmt.Sprintf("Defined mine explision wav(#WAV00) is not used: %s", um.value),
		Message_ja: fmt.Sprintf("定義されている地雷爆発音(#WAV00)は使用されていません: %s", um.value),
	}
}

func CheckSoundOfMineExplosionIsUsed(bmsFile *BmsFile) *unusedMineSound {
	for _, def := range bmsFile.HeaderWav {
		if def.Index == "00" && len(bmsFile.BmsMineObjs) == 0 {
			return &unusedMineSound{value: def.Value}
		}
	}
	return nil
}

// TODO 更にスライスにしてまとめてSubLogにした方が良い？
type duplicateWavs struct {
	measure  int
	position fraction
	wav      string
	objs     []bmsObj //不要？ channel情報だけが個別
	bmsFile  *BmsFile
}

func (dw duplicateWavs) Log() Log {
	str := fmt.Sprintf("#%03d (%d/%d) %s (%s) * %d",
		dw.measure, dw.position.Numerator, dw.position.Denominator, strings.ToUpper(dw.wav),
		dw.bmsFile.definedValue(Wav, strings.ToUpper(dw.wav)), len(dw.objs))
	log := Log{
		Level:      Warning,
		Message:    "Placed WAV objects are duplicate: " + str,
		Message_ja: "WAVオブジェが重複して配置されています: " + str,
		//SubLogs:    []string{}, //[]SubLog{},
	}
	//for _, dupWav := range dw.objs {
	/*log.SubLogs = append(log.SubLogs, SubLog{Message: fmt.Sprintf("#%03d (%d/%d) %s (%s) * %d",
	dw.measure, dw.position.Numerator, dw.position.Denominator, strings.ToUpper(dw.wav),
	dw.bmsFile.definedValue(Wav, strings.ToUpper(dupWav.value36())), len(dw.objs))})*/
	/*log.SubLogs = append(log.SubLogs, fmt.Sprintf("#%03d (%d/%d) %s (%s) * %d",
			dw.measure, dw.position.Numerator, dw.position.Denominator, strings.ToUpper(dw.wav),
			dw.bmsFile.definedValue(Wav, strings.ToUpper(dupWav.value36())), len(dw.objs)))
	}*/
	return log
}

func CheckWavDuplicate(bmsFile *BmsFile) (dupWavs []duplicateWavs) {
	boi := newBmsObjsIterator(bmsFile.BmsWavObjs)
	for momentObjs := boi.next(); len(momentObjs) > 0; momentObjs = boi.next() {
		duplicates := []string{}
		objCounts := map[string]([]bmsObj){}
		for _, obj := range momentObjs {
			if bmsFile.definedValue(Wav, strings.ToUpper(obj.value36())) == "" {
				continue
			}
			if len(objCounts[obj.value36()]) == 1 {
				duplicates = append(duplicates, obj.value36())
			}
			objCounts[obj.value36()] = append(objCounts[obj.value36()], obj)
		}
		if len(duplicates) > 0 {
			fp := fraction{momentObjs[0].Position.Numerator, momentObjs[0].Position.Denominator}
			fp.reduce()
			for _, dup := range duplicates {
				dupWavs = append(dupWavs, duplicateWavs{
					measure:  momentObjs[0].Measure,
					position: fp,
					wav:      dup,
					objs:     objCounts[dup],
					bmsFile:  bmsFile,
				})
			}
		}
	}
	if len(dupWavs) > 0 {
		return dupWavs
	}
	return nil
}

type overlapNotes struct {
	objs []bmsObj
}

func (on overlapNotes) Log() Log {
	fp := fraction{on.objs[0].Position.Numerator, on.objs[0].Position.Denominator}
	fp.reduce()
	objsStr := ""
	for _, obj := range on.objs {
		objsStr += fmt.Sprintf("[%s]#WAV%s ", strings.ToUpper(obj.Channel), strings.ToUpper(obj.value36())) // TODO SubLogにする？
	}
	overlapStr := fmt.Sprintf("#%03d (%d/%d) %s", on.objs[0].Measure, fp.Numerator, fp.Denominator, objsStr)
	return Log{
		Level:      Error,
		Message:    fmt.Sprintf("Placed notes overlap: %s", overlapStr),
		Message_ja: fmt.Sprintf("配置されているノーツが重なり合っています: %s", overlapStr),
	}
}

func CheckNoteOverlap(bmsFile *BmsFile) (ons []overlapNotes) {
	notBgmWavObjs := []bmsObj{}
	for _, obj := range bmsFile.BmsWavObjs {
		if obj.Channel != "01" {
			notBgmWavObjs = append(notBgmWavObjs, obj)
		}
	}
	allNotes := append(notBgmWavObjs, bmsFile.BmsMineObjs...)
	sort.Slice(allNotes, func(i, j int) bool { return allNotes[i].time() < allNotes[j].time() })
	boi := newBmsObjsIterator(allNotes) // TODO イテレータ内でソートすべき？
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
				ons = append(ons, overlapNotes{objs: objs})
			}
		}
	}
	return ons
}

type noteInLN struct {
	containedObj *bmsObj
	lnStart      *bmsObj
}

func (nl noteInLN) Log() Log {
	noteType := "Normal"
	noteType_ja := "通常"
	if nl.containedObj.Channel[0] == 'd' || nl.containedObj.Channel[0] == 'e' {
		noteType = "Mine"
		noteType_ja = "地雷"
	}
	return Log{
		Level:      Error,
		Message:    fmt.Sprintf("%s note is in LN: %s in %s", noteType, nl.containedObj.string(nil), nl.lnStart.string(nil)),
		Message_ja: fmt.Sprintf("%sノーツがLNの中に配置されています: %s in %s", noteType_ja, nl.containedObj.string(nil), nl.lnStart.string(nil)), // TODO inを日本語にする？
	}
}

type endMissingLN struct {
	lnStart *bmsObj
	bmsFile *BmsFile
}

func (el endMissingLN) Log() Log {
	return Log{
		Level:      Error,
		Message:    fmt.Sprintf("End of LN is missing: %s", el.lnStart.string(el.bmsFile)),
		Message_ja: fmt.Sprintf("LNの終端がありません: %s", el.lnStart.string(el.bmsFile)),
	}
}

func CheckEndOfLNExistsAndNotesInLN(bmsFile *BmsFile) (nls []noteInLN, els []endMissingLN) {
	noteObjs := []bmsObj{}
	for _, obj := range bmsFile.BmsWavObjs {
		if matchChannel(obj.Channel, NOTE_CHANNELS) {
			noteObjs = append(noteObjs, obj)
		}
	}
	nmObjs := append(noteObjs, bmsFile.BmsMineObjs...)
	sort.Slice(nmObjs, func(i, j int) bool { return nmObjs[i].time() < nmObjs[j].time() })
	boi := newBmsObjsIterator(nmObjs)
	ongoingLNs := map[string]*bmsObj{}
	// ノーツ内包ログをレーンごとに貯めておき、LN終端が見つかったらログを確定させる
	ongoingLNLogs := map[string]([]Log){}
	commitOngoingLNLogs := func(ch string) {
		for _, log := range ongoingLNLogs[ch] {
			bmsFile.Logs = append(bmsFile.Logs, log)
		}
		ongoingLNLogs[ch] = nil
	}

	for momentObjs := boi.next(); len(momentObjs) > 0; momentObjs = boi.next() {
		for _, obj := range momentObjs {
			chint, _ := strconv.Atoi(obj.Channel)
			if (chint >= 51 && chint <= 59) || (chint >= 61 && chint <= 69) { // LN start and end
				if ongoingLNs[obj.Channel] == nil {
					ongoingLNs[obj.Channel] = &obj
				} else {
					ongoingLNs[obj.Channel] = nil
					commitOngoingLNLogs(obj.Channel)
				}
			} else if (chint >= 11 && chint <= 19) || (chint >= 21 && chint <= 29) { // normal note
				lnCh := strconv.Itoa(chint + 40)
				if ongoingLNs[lnCh] != nil {
					if obj.value36() == bmsFile.lnobj() { // lnobj
						ongoingLNs[lnCh] = nil
						commitOngoingLNLogs(lnCh)
					} else {
						nls = append(nls, noteInLN{containedObj: &obj, lnStart: ongoingLNs[lnCh]})
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
					nls = append(nls, noteInLN{containedObj: &obj, lnStart: ongoingLNs[lnCh]})
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
		els = append(els, endMissingLN{lnStart: lnStart, bmsFile: bmsFile})
	}

	return nls, els
}

type invalidBpmObj struct {
	obj *bmsObj
}

func (ib invalidBpmObj) Log() Log {
	objStr := fmt.Sprintf("%s (#%03d (%d/%d))", strings.ToUpper(ib.obj.value36()), ib.obj.Measure, ib.obj.Position.Numerator, ib.obj.Position.Denominator)
	return Log{
		Level:      Error,
		Message:    fmt.Sprintf("BPM object has invalid value: %s", objStr),
		Message_ja: fmt.Sprintf("BPMオブジェの値が無効です: %s", objStr),
	}
}

func CheckBpmValue(bmsFile *BmsFile) (ibs []invalidBpmObj) {
	for _, obj := range bmsFile.BmsBpmObjs {
		_, err := strconv.ParseInt(obj.value36(), 16, 64)
		if err != nil {
			ibs = append(ibs, invalidBpmObj{obj: &obj})
		}
	}
	return ibs
}

type invalidMeasureLength struct {
	mlen *measureLength
}

func (im invalidMeasureLength) Log() Log {
	return Log{
		Level:      Error,
		Message:    fmt.Sprintf("#%03d measure length has invalid value: %s", im.mlen.Measure, im.mlen.LengthStr),
		Message_ja: fmt.Sprintf("#%03d小節の小節長の値が無効です: %s", im.mlen.Measure, im.mlen.LengthStr),
	}
}

type duplicateMeasureLength struct {
	mlens []measureLength
}

func (dm duplicateMeasureLength) Log() Log {
	lens := dm.mlens[0].LengthStr
	for j := 1; j < len(dm.mlens); j++ {
		lens += ", " + dm.mlens[j].LengthStr
	}
	return Log{
		Level:      Warning,
		Message:    fmt.Sprintf("#%03d measure length is duplicate: %s", dm.mlens[0].Measure, lens),
		Message_ja: fmt.Sprintf("#%03d小節の小節長が重複しています: %s", dm.mlens[0].Measure, lens),
	}
}

func CheckMeasureLength(bmsFile *BmsFile) (ims []invalidMeasureLength, dms []duplicateMeasureLength) {
	duplicateMlens := []measureLength{}
	for i, mlen := range bmsFile.BmsMeasureLengths {
		if mlen.length() <= 0 {
			ims = append(ims, invalidMeasureLength{mlen: &mlen})
		}
		duplicateMlens = append(duplicateMlens, mlen)
		if i == len(bmsFile.BmsMeasureLengths)-1 || mlen.Measure != bmsFile.BmsMeasureLengths[i+1].Measure {
			if len(duplicateMlens) > 1 {
				dms = append(dms, duplicateMeasureLength{mlens: duplicateMlens})
			}
			duplicateMlens = []measureLength{}
		}
	}
	return ims, dms
}

type withoutKeysoundMoments struct {
	wavFileIsExist   bool
	noWavMomentCount int // CountではなくnoWavMoment[][]bmsObjにする？
	momentCount      int
}

func (wm withoutKeysoundMoments) Log() Log {
	audioText := ""
	audioText_ja := ""
	if wm.wavFileIsExist {
		audioText = " (or audio file)"
		audioText_ja = "(または音声ファイル)"
	}
	momentText := fmt.Sprintf("%.1f%%(%d/%d)", float64(wm.noWavMomentCount)/float64(wm.momentCount)*100, wm.noWavMomentCount, wm.momentCount)
	return Log{
		Level:      Warning,
		Message:    fmt.Sprintf("Moments without keysound%s exist: %s", audioText, momentText),
		Message_ja: fmt.Sprintf("キー音%sの無い瞬間があります: %s", audioText_ja, momentText),
	}
}

type withoutKeysoundNotes struct {
	wavFileIsExist  bool
	noWavObjs       []bmsObj
	noWavLnObjs     []bmsObj
	totalNotesCount int
}

func (wn withoutKeysoundNotes) Log() Log {
	audioText := ""
	audioText_ja := ""
	if wn.wavFileIsExist {
		audioText = " (or audio file)"
		audioText_ja = "(または音声ファイル)"
	}
	notesText := fmt.Sprintf("%.1f%%(%d/%d)",
		float64(len(wn.noWavObjs)+len(wn.noWavLnObjs)/2)/float64(wn.totalNotesCount)*100, len(wn.noWavObjs)+len(wn.noWavLnObjs)/2, wn.totalNotesCount)
	return Log{
		Level:      Notice,
		Message:    fmt.Sprintf("Notes without keysound%s exist: %s", audioText, notesText),
		Message_ja: fmt.Sprintf("キー音%sの無いノーツがあります: %s", audioText_ja, notesText),
	}
}

// 実在しないファイル名を持つWAV定義を考慮する場合、wavFileIsExistを渡す。そうでなければnilを渡す。
func CheckWithoutKeysound(bmsFile *BmsFile, wavFileIsExist func(string) bool) (wm *withoutKeysoundMoments, wn *withoutKeysoundNotes) {
	noteObjs := []bmsObj{}
	for _, obj := range bmsFile.BmsWavObjs {
		if matchChannel(obj.Channel, NOTE_CHANNELS) {
			noteObjs = append(noteObjs, obj)
		}
	}

	momentCount := 0
	noWavMomentCount := 0
	var noWavObjs []bmsObj
	var noWavLnObjs []bmsObj
	nonexistentWavIsPlaced := false
	boi := newBmsObjsIterator(noteObjs)
	for momentObjs := boi.next(); len(momentObjs) > 0; momentObjs = boi.next() {
		momentCount++
		isNoKeysoundMoment := true
		for _, momObj := range momentObjs {
			isNoKeysoundNote := true
			if momObj.value36() == bmsFile.lnobj() {
				isNoKeysoundNote = false
				isNoKeysoundMoment = false
			} else {
				for _, wav := range bmsFile.HeaderWav {
					if momObj.value36() == wav.Index && (wavFileIsExist == nil || (wavFileIsExist != nil && wavFileIsExist(wav.Value))) {
						isNoKeysoundNote = false
						isNoKeysoundMoment = false
						break
					}
					if momObj.value36() == wav.Index && (wavFileIsExist != nil && !wavFileIsExist(wav.Value)) {
						nonexistentWavIsPlaced = true
					}
				}
			}
			if isNoKeysoundNote {
				if matchChannel(momObj.Channel, LN_CHANNELS) {
					noWavLnObjs = append(noWavLnObjs, momObj)
				} else {
					noWavObjs = append(noWavObjs, momObj)
				}
			}
		}
		if isNoKeysoundMoment {
			noWavMomentCount++
		}
	}

	// 実在しないWAVファイルの定義はあるが、その定義が配置されていない場合は、実在しないWAV定義が無い(or考慮しない)場合と結果が変わらないので、ログを出す意味が無い
	if wavFileIsExist != nil && !nonexistentWavIsPlaced {
		return nil, nil
	}

	if noWavMomentCount > 0 {
		wm = &withoutKeysoundMoments{wavFileIsExist: wavFileIsExist != nil, noWavMomentCount: noWavMomentCount, momentCount: momentCount}
	}
	if len(noWavObjs)+len(noWavLnObjs) > 0 {
		wn = &withoutKeysoundNotes{wavFileIsExist: wavFileIsExist != nil, noWavObjs: noWavObjs, noWavLnObjs: noWavLnObjs, totalNotesCount: bmsFile.TotalNotes}
	}
	return wm, wn
}
