package checkbms

import (
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/Shimi9999/checkbms/bmson"
)

func (bmsonFile *BmsonFile) ScanBmsonFile() error {
	if bmsonFile.FullText == nil {
		return fmt.Errorf("FullText is empty: %s", bmsonFile.Path)
	}

	bmsonData, logs, err := ScanBmson(bmsonFile.FullText)
	bmsonFile.Logs = logs
	if err != nil {
		bmsonFile.IsInvalid = true
		return err
	}
	bmsonFile.Bmson = *bmsonData

	// getKeymode
	func() {
		if keymodeMatch := regexp.MustCompile(`-(\d+)k`).FindStringSubmatch(bmsonData.Info.Mode_hint); keymodeMatch != nil {
			keymode, _ := strconv.Atoi(keymodeMatch[1])
			bmsonFile.Keymode = keymode
		}
	}()

	// countTotalNotes
	func() {
		notesMap := map[string]bmson.Note{}
		for _, ch := range bmsonData.Sound_channels {
			for _, note := range ch.Notes {
				x, isFloat64 := note.X.(float64)
				if isFloat64 && x-math.Floor(x) == 0 && x > 0 && !note.Up {
					key := fmt.Sprintf("%d-%d", int(x), note.Y)
					notesMap[key] = note
				}
			}
		}
		totalNotes := len(notesMap)
		for _, note := range notesMap {
			if note.L > 0 && (note.T >= 2 || note.T != 1 && bmsonData.Info.Ln_type >= 2) {
				totalNotes++
			}
		}
		bmsonFile.TotalNotes = totalNotes
	}()

	return nil
}

type invalidField struct {
	fieldName    string
	value        interface{}
	locationName string
}

func (i invalidField) Log() Log {
	val := i.value
	if str, isString := val.(string); isString {
		val = fmt.Sprintf("\"%s\"", str)
	}
	return Log{
		Level:      Warning,
		Message:    fmt.Sprintf("Invalid field name: {\"%s\": %v} in %s", i.fieldName, val, i.locationName),
		Message_ja: fmt.Sprintf("無効なフィールド名です: {\"%s\": %v} in %s", i.fieldName, val, i.locationName),
	}
}

// 型エラー 想定する型と実際の型も表示する？
type invalidType struct {
	fieldName string
	value     interface{}
}

func (i invalidType) Log() Log {
	return Log{
		Level:      Error,
		Message:    fmt.Sprintf("%s has invalid value: %v", i.fieldName, i.value),
		Message_ja: fmt.Sprintf("%sが無効な値です: %v", i.fieldName, i.value),
	}
}

func ScanBmson(bytes []byte) (bmsonData *bmson.Bmson, logs Logs, _ error) {
	var bmsonObj interface{}
	if err := json.Unmarshal(bytes, &bmsonObj); err != nil {
		logs = append(logs, Log{
			Level:      Error,
			Message:    fmt.Sprintf("Invalid bmson format: %s", err.Error()),
			Message_ja: fmt.Sprintf("bmsonのフォーマットが無効です: %s", err.Error()),
		})
		for _, log := range logs {
			fmt.Println(log.String())
		}
		return nil, logs, err
	}

	ifs := []invalidField{}
	its := []invalidType{}

	// bmsonのフォーマットチェックをしながら、bmsonデータを読み込む
	// TODO unmershallのときにmapにまとめられてしまうので、キーの重複が検知できない
	var load func(jsonObj interface{}, dataType reflect.Type, keyName string) reflect.Value
	load = func(jsonObj interface{}, dataType reflect.Type, keyName string) reflect.Value {
		if dataType.Kind() == reflect.Invalid {
			return reflect.Value{}
		}

		isPtr := false
		if dataType.Kind() == reflect.Ptr {
			dataType = dataType.Elem()
			isPtr = true
		}

		switch dataType.Kind() {
		case reflect.Struct:
			if mapVals, isMap := jsonObj.(map[string]interface{}); isMap {
				structData := reflect.New(dataType).Elem()
				dataType := structData.Type()
				for key, val := range mapVals {
					match := false
					for i := 0; i < dataType.NumField(); i++ {
						fieldName := strings.ToLower(dataType.Field(i).Name)
						if key == fieldName {
							match = true
							loadedValue := load(val, dataType.Field(i).Type, fmt.Sprintf("%s.%s", keyName, key))
							if loadedValue.Kind() == reflect.Ptr && dataType.Field(i).Type.Kind() != reflect.Ptr {
								loadedValue = loadedValue.Elem()
							}
							structData.Field(i).Set(loadedValue)
							break
						}
					}
					if !match {
						ifs = append(ifs, invalidField{key, val, keyName})
					}
				}
				if isPtr && structData.Kind() != reflect.Ptr {
					return structData.Addr()
				}
				return structData
			} else {
				its = append(its, invalidType{keyName, jsonObj})
			}
		case reflect.Slice:
			if sliceVals, isSlice := jsonObj.([]interface{}); isSlice {
				sliceData := reflect.MakeSlice(dataType, len(sliceVals), len(sliceVals))
				for i, val := range sliceVals {
					loadedValue := load(val, dataType.Elem(), fmt.Sprintf("%s[%d]", keyName, i))
					if loadedValue.Kind() == reflect.Ptr && dataType.Elem().Kind() != reflect.Ptr {
						sliceData.Index(i).Set(loadedValue.Elem())
					} else {
						sliceData.Index(i).Set(loadedValue)
					}
				}
				if isPtr && sliceData.Kind() != reflect.Ptr {
					return sliceData.Addr()
				}
				return sliceData
			} else {
				its = append(its, invalidType{keyName, jsonObj})
			}
		case reflect.String:
			if val, ok := jsonObj.(string); ok {
				return reflect.ValueOf(&val)
			} else {
				its = append(its, invalidType{keyName, jsonObj})
			}
		case reflect.Int:
			if val, ok := jsonObj.(float64); ok {
				if val-math.Floor(val) != 0 {
					its = append(its, invalidType{keyName, jsonObj})
				}
				iVal := int(val)
				return reflect.ValueOf(&iVal)
			} else {
				its = append(its, invalidType{keyName, jsonObj})
			}
		case reflect.Float64:
			if val, ok := jsonObj.(float64); ok {
				return reflect.ValueOf(&val)
			} else {
				its = append(its, invalidType{keyName, jsonObj})
			}
		case reflect.Bool:
			if val, ok := jsonObj.(bool); ok {
				return reflect.ValueOf(&val)
			} else {
				its = append(its, invalidType{keyName, jsonObj})
			}
		case reflect.Interface:
			return reflect.ValueOf(&jsonObj)
		}

		return reflect.Value{}
	}

	bmsonData = load(bmsonObj, reflect.TypeOf(bmsonData), "root").Interface().(*bmson.Bmson)

	for _, _if := range ifs {
		logs = append(logs, _if.Log())
	}
	for _, it := range its {
		logs = append(logs, it.Log())
	}

	return bmsonData, logs, nil
}

func CheckBmsonFile(bmsonFile *BmsonFile) {
	if logs := CheckBmsonInfo(bmsonFile); len(logs) > 0 {
		bmsonFile.Logs = append(bmsonFile.Logs, logs...)
	}

	for _, result := range CheckTitleTextsAreDuplicate(bmsonFile) {
		bmsonFile.Logs = append(bmsonFile.Logs, result.Log())
	}

	for _, result := range CheckSoundChannelNameIsInvalid(bmsonFile) {
		bmsonFile.Logs = append(bmsonFile.Logs, result.Log())
	}

	if result := CheckNoWavSoundChannels(bmsonFile); result != nil {
		bmsonFile.Logs = append(bmsonFile.Logs, result.Log())
	}

	if result := CheckTotalnotesIsZero(&bmsonFile.BmsFileBase); result != nil {
		bmsonFile.Logs = append(bmsonFile.Logs, result.Log())
	}

	if result := CheckSoundNotesIn0thMeasure(bmsonFile); result != nil {
		bmsonFile.Logs = append(bmsonFile.Logs, result.Log())
	}

	for _, result := range CheckPlacedUndefiedBgaIds(bmsonFile) {
		bmsonFile.Logs = append(bmsonFile.Logs, result.Log())
	}

	if result := CheckDefinedUnplacedBgaHeader(bmsonFile); result != nil {
		bmsonFile.Logs = append(bmsonFile.Logs, result.Log())
	}
}

var infoFields = []Command{
	{"title", String, Necessary, nil},
	{"subtitle", String, Unnecessary, nil},
	{"artist", String, Semi_necessary, nil},
	{"subartists", String, Unnecessary, nil}, // Strings型用意する?
	{"genre", String, Semi_necessary, nil},
	{"mode_hint", String, Semi_necessary, []string{`^beat-\d+k$`, `^popn-\d+k$`, `^keyboard-\d+k$`, `generic-\d+keys$`}},
	{"chart_name", String, Unnecessary, nil},
	{"level", Int, Semi_necessary, []int{0, math.MaxInt64}},
	{"init_bpm", Float, Necessary, []float64{math.SmallestNonzeroFloat64, math.MaxFloat64}},
	{"judge_rank", Float, Semi_necessary, []float64{math.SmallestNonzeroFloat64, math.MaxFloat64}},
	{"total", Float, Semi_necessary, []float64{0, math.MaxFloat64}},
	{"back_image", Path, Unnecessary, IMAGE_EXTS},
	{"eyecatch_image", Path, Unnecessary, IMAGE_EXTS},
	{"title_image", Path, Unnecessary, IMAGE_EXTS},
	{"banner_image", Path, Unnecessary, IMAGE_EXTS},
	{"preview_music", Path, Unnecessary, AUDIO_EXTS},
	{"resolution", Int, Unnecessary, []int{1, math.MaxInt64}},
	{"ln_type", Int, Unnecessary, []int{0, 3}},
}

func CheckBmsonInfo(bmsonFile *BmsonFile) (logs Logs) {
	iv := reflect.ValueOf(bmsonFile.Info).Elem()
	it := iv.Type()
	for i := 0; i < iv.NumField(); i++ {
		ft := it.Field(i)
		fv := iv.Field(i)
		keyName := strings.ToLower(ft.Name)

		isEmptyValue := func(val reflect.Value) bool {
			return ft.Type.Kind() == reflect.String && fv.String() == ""
		}

		isInvalidValue := func(val reflect.Value) (_ bool, valStr string) {
			switch ft.Type.Kind() {
			case reflect.String:
				valStr = fv.String()
			case reflect.Int:
				valStr = strconv.Itoa(int(fv.Int()))
			case reflect.Float64:
				valStr = strconv.FormatFloat(fv.Float(), 'f', -1, 64)
			case reflect.Slice:
				for j := 0; j < fv.Len(); j++ {
					if fv.Index(j).Type().Kind() == reflect.String {
						if valStr != "" {
							valStr += " "
						}
						valStr += fv.Index(j).String()
					}
				}
			}
			isInRange, err := infoFields[i].isInRange(valStr)
			return err != nil || !isInRange, valStr
		}

		if isEmptyValue(fv) {
			if infoFields[i].Necessity != Unnecessary {
				logs = append(logs, Log{
					Level:      Warning,
					Message:    fmt.Sprintf("info.%s value is empty", keyName),
					Message_ja: fmt.Sprintf("info.%sの値が空です", keyName),
				})
			}
		} else if is, valStr := isInvalidValue(fv); is {
			logs = append(logs, Log{
				Level:      Error,
				Message:    fmt.Sprintf("info.%s has invalid value: %s", keyName, valStr),
				Message_ja: fmt.Sprintf("info.%sが無効な値です: %s", keyName, valStr),
			})
		} else if keyName == "judge_rank" {
			judgeRank := fv.Float()
			if judgeRank >= 125 {
				logs = append(logs, Log{
					Level:      Notice,
					Message:    fmt.Sprintf("info.judge_rank is very high: %s", formatFloat(judgeRank)),
					Message_ja: fmt.Sprintf("info.judge_rankがかなり高いです: %s", formatFloat(judgeRank)),
				})
			} else if judgeRank <= 50 {
				logs = append(logs, Log{
					Level:      Notice,
					Message:    fmt.Sprintf("info.judge_rank is very low: %s", formatFloat(judgeRank)),
					Message_ja: fmt.Sprintf("info.judge_rankがかなり低いです: %s", formatFloat(judgeRank)),
				})
			}
		} else if keyName == "total" {
			bmsonTotal := fv.Float()
			total := CalculateDefaultTotal(bmsonFile.TotalNotes, bmsonFile.Keymode) * bmsonTotal / 100
			if total < 100 {
				logs = append(logs, Log{
					Level:      Warning,
					Message:    fmt.Sprintf("Real total value is under 100: Real:%.2f Defined:%s", total, formatFloat(bmsonTotal)),
					Message_ja: fmt.Sprintf("実際のTotal値が100未満です: 実際:%.2f 定義:%s", total, formatFloat(bmsonTotal)),
				})
			} else if bmsonFile.TotalNotes > 0 {
				totalJudge := JudgeOverTotal(total, bmsonFile.TotalNotes, bmsonFile.Keymode)
				if totalJudge > 0 {
					logs = append(logs, Log{
						Level:      Notice,
						Message:    fmt.Sprintf("info.total is very high(TotalNotes=%d): Real:%.2f Defined:%s", bmsonFile.TotalNotes, total, formatFloat(bmsonTotal)),
						Message_ja: fmt.Sprintf("info.totalがかなり高いです(トータルノーツ=%d): 実際:%.2f 定義:%s", bmsonFile.TotalNotes, total, formatFloat(bmsonTotal)),
					})
				} else if totalJudge < 0 {
					logs = append(logs, Log{
						Level:      Notice,
						Message:    fmt.Sprintf("info.total is very low(TotalNotes=%d): Real:%.2f Defined:%s", bmsonFile.TotalNotes, total, formatFloat(bmsonTotal)),
						Message_ja: fmt.Sprintf("info.totalがかなり低いです(トータルノーツ=%d): 実際:%.2f 定義:%s", bmsonFile.TotalNotes, total, formatFloat(bmsonTotal)),
					})
				}
			}
		}
	}

	return logs
}

type titleTextsAreDuplicate struct {
	title1, title2         string
	fieldName1, fieldName2 string
}

func (t titleTextsAreDuplicate) Log() Log {
	return Log{
		Level:      Warning,
		Message:    fmt.Sprintf("info.%s and info.%s contain the same string: %s, %s", t.fieldName1, t.fieldName2, t.title1, t.title2),
		Message_ja: fmt.Sprintf("info.%sとinfo.%sが同じ文字列を含んでいます: %s, %s", t.fieldName1, t.fieldName2, t.title1, t.title2),
	}
}

func CheckTitleTextsAreDuplicate(bmsonFile *BmsonFile) (tds []titleTextsAreDuplicate) {
	titles := []string{bmsonFile.Info.Title, bmsonFile.Info.Subtitle, bmsonFile.Info.Chart_name}
	fieldNames := []string{"title", "subtitle", "chart_name"}
	for i := 0; i < len(titles); i++ {
		for j := i + 1; j < len(titles); j++ {
			if titles[i] != "" && titles[j] != "" && strings.HasSuffix(strings.ToLower(titles[i]), strings.ToLower(titles[j])) {
				td := titleTextsAreDuplicate{
					title1:     titles[i],
					title2:     titles[j],
					fieldName1: fieldNames[i],
					fieldName2: fieldNames[j],
				}
				tds = append(tds, td)
			}
		}
	}
	return tds
}

type invalidSoundChannelName struct {
	name  string
	index int
}

func (i invalidSoundChannelName) Log() Log {
	return Log{
		Level:      Warning,
		Message:    fmt.Sprintf("sound_channels[%d].name is invalid value: %s", i.index, i.name),
		Message_ja: fmt.Sprintf("sound_channels[%d].nameが無効な値です: %s", i.index, i.name),
	}
}

func CheckSoundChannelNameIsInvalid(bmsonFile *BmsonFile) (iss []invalidSoundChannelName) {
	for i, soundChannel := range bmsonFile.Sound_channels {
		if !hasExts(soundChannel.Name, AUDIO_EXTS) {
			iss = append(iss, invalidSoundChannelName{name: soundChannel.Name, index: i})
		}
	}
	return iss
}

type noWavSoundChannels struct {
	noWavSoundChannels []bmson.SoundChannel
}

func (nw noWavSoundChannels) Log() Log {
	return Log{
		Level: Notice,
		Message: fmt.Sprintf("sound_channels has filenames non-.wav extension(*%d): %s etc...",
			len(nw.noWavSoundChannels), nw.noWavSoundChannels[0].Name),
		Message_ja: fmt.Sprintf("sound_channelsに拡張子.wavでないファイル名があります(*%d): %s etc...",
			len(nw.noWavSoundChannels), nw.noWavSoundChannels[0].Name),
	}
}

func CheckNoWavSoundChannels(bmsonFile *BmsonFile) (nw *noWavSoundChannels) {
	noWavs := []bmson.SoundChannel{}
	for _, soundChannel := range bmsonFile.Sound_channels {
		if strings.ToLower(filepath.Ext(soundChannel.Name)) != ".wav" {
			noWavs = append(noWavs, soundChannel)
		}
	}
	if len(noWavs) > 0 {
		nw = &noWavSoundChannels{noWavSoundChannels: noWavs}
	}
	return nw
}

type soundNote struct {
	fileName string
	note     bmson.Note
}

type soundNotesIn0thMeasure struct {
	soundNotes []soundNote
}

func (sn soundNotesIn0thMeasure) Log() Log {
	log := Log{
		Level:      Warning,
		Message:    "Note exists in 0th measure",
		Message_ja: "0小節目にノーツが配置されています",
		SubLogs:    []string{},
		SubLogType: List,
	}
	for _, soundNote := range sn.soundNotes {
		log.SubLogs = append(log.SubLogs, fmt.Sprintf("(x:%v, y:%d) %s", soundNote.note.X, soundNote.note.Y, soundNote.fileName))
	}
	return log
}

func CheckSoundNotesIn0thMeasure(bmsonFile *BmsonFile) *soundNotesIn0thMeasure {
	firstBarY := 0
	for _, line := range bmsonFile.Lines { // ソートが必要？
		if line.Y > 0 {
			firstBarY = line.Y
			break
		}
	}
	detectedSoundNotes := []soundNote{}
	for _, soundChannel := range bmsonFile.Sound_channels {
		for _, note := range soundChannel.Notes {
			if x, ok := note.X.(float64); ok && x > 0 && note.Y < firstBarY {
				detectedSoundNotes = append(detectedSoundNotes, soundNote{fileName: soundChannel.Name, note: note})
			}
		}
	}
	if len(detectedSoundNotes) > 0 {
		return &soundNotesIn0thMeasure{soundNotes: detectedSoundNotes}
	}
	return nil
}

type placedUndefinedBgaIds struct {
	eventName string
	ids       []int
}

func (b placedUndefinedBgaIds) Log() Log {
	log := Log{
		Level:      Warning,
		Message:    fmt.Sprintf("Placed %s.id is undefined", b.eventName),
		Message_ja: fmt.Sprintf("配置されている%s.idが未定義です", b.eventName),
		SubLogs:    []string{},
		SubLogType: List,
	}
	for _, id := range b.ids {
		log.SubLogs = append(log.SubLogs, fmt.Sprintf("%d", id))
	}
	return log
}

func CheckPlacedUndefiedBgaIds(bmsonFile *BmsonFile) (pus []placedUndefinedBgaIds) {
	if bmsonFile.Bga == nil {
		return nil
	}

	bgaEventss := [][]bmson.BGAEvent{bmsonFile.Bga.Bga_events, bmsonFile.Bga.Layer_events, bmsonFile.Bga.Poor_events}
	bgaEventsNames := []string{"bga_events", "layer_events", "poor_events"}
	for i, bgaEvents := range bgaEventss {
		undefinedBgaIds := []int{}
		for _, bgaEvent := range bgaEvents {
			defined := false
			for _, header := range bmsonFile.Bga.Bga_header {
				if bgaEvent.Id == header.Id {
					defined = true
					break
				}
			}
			if !defined {
				undefinedBgaIds = append(undefinedBgaIds, bgaEvent.Id)
			}
		}
		if len(undefinedBgaIds) > 0 {
			pus = append(pus, placedUndefinedBgaIds{eventName: bgaEventsNames[i], ids: undefinedBgaIds})
		}
	}
	return pus
}

type definedUnplacedBgaHeader struct {
	headers []bmson.BGAHeader
}

func (b definedUnplacedBgaHeader) Log() Log {
	log := Log{
		Level:      Warning,
		Message:    "Defined bga_header is not placed",
		Message_ja: "定義されているbga_headerが未配置です",
		SubLogs:    []string{},
		SubLogType: List,
	}
	for _, header := range b.headers {
		log.SubLogs = append(log.SubLogs, fmt.Sprintf("id:%d, name:%s", header.Id, header.Name))
	}
	return log
}

func CheckDefinedUnplacedBgaHeader(bmsonFile *BmsonFile) (du *definedUnplacedBgaHeader) {
	if bmsonFile.Bga == nil {
		return nil
	}

	unplacedBgaHeaders := []bmson.BGAHeader{}
	bgaEventss := [][]bmson.BGAEvent{bmsonFile.Bga.Bga_events, bmsonFile.Bga.Layer_events, bmsonFile.Bga.Poor_events}
	for _, header := range bmsonFile.Bga.Bga_header {
		placed := false
		for _, bgaEvents := range bgaEventss {
			for _, bgaEvent := range bgaEvents {
				if header.Id == bgaEvent.Id {
					placed = true
					break
				}
			}
			if placed {
				break
			}
		}
		if !placed {
			unplacedBgaHeaders = append(unplacedBgaHeaders, header)
		}
	}
	if len(unplacedBgaHeaders) > 0 {
		du = &definedUnplacedBgaHeader{headers: unplacedBgaHeaders}
	}
	return du
}

// 小数点以下の無駄な0を消去して整える
func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}
