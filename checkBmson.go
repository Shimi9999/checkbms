package checkbms

import (
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/Shimi9999/checkbms/bmson"
	"github.com/buger/jsonparser"
)

type _Bmson struct {
	Version        *string         `json:"version" validate:"required"`
	Info           *_BmsonInfo     `json:"info" validate:"required"`
	Lines          []_BarLine      `json:"lines"`
	Bpm_events     []_BpmEvent     `json:"bpm_events"`
	Stop_events    []_StopEvent    `json:"stop_events"`
	Sound_channels []_SoundChannel `json:"sound_channels" validate:"required"`
	Bga            *_BGA           `json:"bga"`
	Scroll_events  []_ScrollEvent  `json:"scroll_events"` // beatoraja expansion
}

type _BmsonInfo struct {
	Title          *string  `json:"title"`
	Subtitle       *string  `json:"subtitle"`
	Artist         *string  `json:"artist"`
	Subartists     []string `json:"subartists"`
	Genre          *string  `json:"genre"`
	Mode_hint      *string  `json:"mode_hint"`
	Chart_name     *string  `json:"chart_name"`
	Level          *int     `json:"level"`
	Init_bpm       *float64 `json:"init_bpm" validate:"required"`
	Judge_rank     *float64 `json:"judge_rank"`
	Total          *float64 `json:"total"`
	Back_image     *string  `json:"back_image"`
	Eyecatch_image *string  `json:"eyecatch_image"`
	Title_image    *string  `json:"title_image"`
	Banner_image   *string  `json:"banner_image"`
	Preview_music  *string  `json:"preview_music"`
	Resolution     *int     `json:"resolution"`
	Ln_type        *int     `json:"ln_type"` // beatoraja expansion
}

type _BarLine struct {
	Y *int `json:"y" validate:"required"`
}

type _SoundChannel struct {
	Name  *string `json:"name" validate:"required"`
	Notes []_Note `json:"notes" validate:"required"`
}

type _Note struct {
	X  *interface{} `json:"x"`
	Y  *int         `json:"y" validate:"required"`
	L  *int         `json:"l"`
	C  *bool        `json:"c" validate:"required"`
	T  *int         `json:"t"`  // beatoraja expansion
	Up *bool        `json:"up"` // beatoraja expansion
}

type _BpmEvent struct {
	Y   *int     `json:"y" validate:"required"`
	Bpm *float64 `json:"bpm" validate:"required"`
}

type _StopEvent struct {
	Y        *int `json:"y" validate:"required"`
	Duration *int `json:"duration" validate:"required"`
}

type _BGA struct {
	Bga_header   []_BGAHeader `json:"bga_header"`
	Bga_events   []_BGAEvent  `json:"bga_events"`
	Layer_events []_BGAEvent  `json:"layer_events"`
	Poor_events  []_BGAEvent  `json:"poor_events"`
}

type _BGAHeader struct {
	Id   *int    `json:"id" validate:"required"`
	Name *string `json:"name" validate:"required"`
}

type _BGAEvent struct {
	Y  *int `json:"y" validate:"required"`
	Id *int `json:"id" validate:"required"`
}

type _ScrollEvent struct { // beatoraja expansion
	Y    *int     `json:"y" validate:"required"`
	Rate *float64 `json:"rate" validate:"required"`
}

func (bmsonFile *BmsonFile) ScanBmsonFile() error {
	if bmsonFile.FullText == nil {
		return fmt.Errorf("FullText is empty: %s", bmsonFile.Path)
	}

	bmsonData, logs, err := ScanBmson(bmsonFile.FullText)
	bmsonFile.Logs = logs
	if err != nil {
		bmsonFile.IsInvalid = true
		return nil
	}
	bmsonFile.Bmson = *bmsonData

	// getKeymode
	func() {
		switch bmsonData.Info.Mode_hint {
		case "keyboard-24k-double":
			bmsonFile.Keymode = 48
		default:
			if keymodeMatch := regexp.MustCompile(`-(\d+)k`).FindStringSubmatch(bmsonData.Info.Mode_hint); keymodeMatch != nil {
				keymode, _ := strconv.Atoi(keymodeMatch[1])
				bmsonFile.Keymode = keymode
			} else {
				// デフォルトはbeat-7k
				// TODO 未指定ならエラーログを出すべき？
				bmsonFile.Keymode = 7
			}
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

	bmsonFile.Logs.addResultLogs(CheckAndDeleteOutOfLaneNotes(bmsonFile))

	return nil
}


type outOfLaneNotes struct {
	soundNotes []soundNote
	mode_hint  string
}

func (n outOfLaneNotes) Log() Log {
	log := Log{
		Level:      Warning,
		Message:    "note.x is out of lane range",
		Message_ja: "ノーツのx位置がレーンの範囲外です",
		SubLogs:    []string{},
		SubLogType: List,
	}
	for _, soundNote := range n.soundNotes {
		log.SubLogs = append(log.SubLogs, soundNote.string())
	}
	return log
}

func isXInLane(x, keymode int) bool {
	switch keymode {
	case 5:
		return x >= 1 && x <= 5 || x == 8
	case 7:
		return x >= 1 && x <= 8
	case 10:
		return x >= 1 && x <= 5 || x >= 8 && x <= 13 || x == 16
	case 14:
		return x >= 1 && x <= 16
	case 9:
		return x >= 1 && x <= 9
	case 24:
		return x >= 1 && x <= 26
	case 48:
		return x >= 1 && x <= 52
	}
	return false
}

func CheckAndDeleteOutOfLaneNotes(bmsonFile *BmsonFile) (on *outOfLaneNotes) {
	outNotes := []soundNote{}
	for ci, soundChannel := range bmsonFile.Sound_channels {
		okNotes := []bmson.Note{}
		for ni, note := range soundChannel.Notes {
			if x, ok := note.X.(float64); ok && x-math.Floor(x) == 0 && (x == 0 || isXInLane(int(x), bmsonFile.Keymode)) {
				okNotes = append(okNotes, note)
			} else {
				outNotes = append(outNotes, soundNote{
					fileName: soundChannel.Name, channelIndex: ci, note: note, noteIndex: ni})
			}
			bmsonFile.Sound_channels[ci].Notes = append([]bmson.Note{}, okNotes...)
		}
	}
	if len(outNotes) > 0 {
		on = &outOfLaneNotes{soundNotes: outNotes, mode_hint: bmsonFile.Info.Mode_hint}
	}
	return on
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

type duplicateField struct {
	fieldName string
	values    []string
}

func (d duplicateField) Log() Log {
	log := Log{
		Level:      Warning,
		Message:    fmt.Sprintf("Duplicate field: %s * %d", d.fieldName, len(d.values)),
		Message_ja: fmt.Sprintf("フィールドが重複しています: %s * %d", d.fieldName, len(d.values)),
		SubLogs:    []string{},
		SubLogType: Detail,
	}
	for _, value := range d.values {
		if len(value) > 100 {
			value = value[:96] + " ... " + value[len(value)-4:]
		}
		log.SubLogs = append(log.SubLogs, value)
	}
	return log
}

func unmarshalBmson(bytes []byte) (_bmson *_Bmson, ifs []invalidField, its []invalidType, dfs []duplicateField, _ error) {
	invalidTypeLog := func(fieldName string, value []byte) {
		its = append(its, invalidType{fieldName: fieldName, value: string(value)})
	}

	var doObjectEach func(data []byte, bmsonType reflect.Type, fieldName string) reflect.Value
	var doArrayEach func(data []byte, bmsonType reflect.Type, fieldName string) reflect.Value

	getValue := func(value []byte, fieldName string, dataType jsonparser.ValueType, bmsonType reflect.Type) reflect.Value {
		expectedType := bmsonType
		if bmsonType.Kind() == reflect.Ptr && bmsonType.Kind() != reflect.Slice {
			expectedType = expectedType.Elem()
		}

		var returnValue reflect.Value
		switch expectedType.Kind() {
		case reflect.Struct:
			if dataType == jsonparser.Object {
				returnValue = doObjectEach(value, bmsonType, fieldName)
			} else {
				invalidTypeLog(fieldName, value)
			}
		case reflect.Slice:
			if dataType == jsonparser.Array {
				returnValue = doArrayEach(value, bmsonType, fieldName)
			} else {
				invalidTypeLog(fieldName, value)
			}
		case reflect.String:
			if dataType == jsonparser.String {
				str := string(value)
				returnValue = reflect.ValueOf(&str)
			} else {
				invalidTypeLog(fieldName, value)
			}
		case reflect.Int:
			if dataType == jsonparser.Number && !strings.Contains(string(value), ".") {
				intNum, _ := strconv.Atoi(string(value))
				returnValue = reflect.ValueOf(&intNum)
			} else {
				invalidTypeLog(fieldName, value)
			}
		case reflect.Float64:
			if dataType == jsonparser.Number {
				floatNum, _ := strconv.ParseFloat(string(value), 64)
				returnValue = reflect.ValueOf(&floatNum)
			} else {
				invalidTypeLog(fieldName, value)
			}
		case reflect.Bool:
			if dataType == jsonparser.Boolean {
				b, _ := strconv.ParseBool(string(value))
				returnValue = reflect.ValueOf(&b)
			} else {
				invalidTypeLog(fieldName, value)
			}
		case reflect.Interface:
			var inter interface{}
			switch dataType {
			case jsonparser.Object:
				inter = doObjectEach(value, bmsonType, fieldName).Elem().Interface()
			case jsonparser.Array:
				inter = doArrayEach(value, bmsonType, fieldName).Interface()
			case jsonparser.String:
				inter = string(value)
			case jsonparser.Number:
				inter, _ = strconv.ParseFloat(string(value), 64)
			case jsonparser.Boolean:
				inter, _ = strconv.ParseBool(string(value))
			}
			returnValue = reflect.ValueOf(&inter)
		}

		return returnValue
	}

	doObjectEach = func(data []byte, bmsonType reflect.Type, fieldName string) reflect.Value {
		var structVal reflect.Value
		if bmsonType.Kind() == reflect.Struct {
			structVal = reflect.New(bmsonType).Elem()
		} else if bmsonType.Kind() == reflect.Ptr && bmsonType.Elem().Kind() == reflect.Struct {
			structVal = reflect.New(bmsonType.Elem()).Elem()
		} else {
			return reflect.Value{}
		}

		keyMap := map[string][]string{}
		keyList := []string{}
		jsonparser.ObjectEach(data, func(key []byte, value []byte, dataType jsonparser.ValueType, offset int) error {
			keyStr := fieldName + "." + string(key)

			fieldKey := strings.ToUpper(string(key)[:1]) + string(key)[1:]
			fieldVal := structVal.FieldByName(fieldKey)
			field, ok := structVal.Type().FieldByName(fieldKey)
			if !ok {
				// 未定義フィールドエラー
				ifs = append(ifs, invalidField{string(key), string(value), fieldName})
				return nil
			}

			if len(keyMap[keyStr]) == 0 {
				keyList = append(keyList, keyStr)
			}
			keyMap[keyStr] = append(keyMap[keyStr], string(value))

			if field.Type.Kind() == reflect.Ptr {
				elemType := field.Type.Elem()
				v := getValue(value, keyStr, dataType, elemType)
				if v.IsValid() {
					fieldVal.Set(v)
				}
			} else {
				v := getValue(value, keyStr, dataType, field.Type)
				if v.IsValid() {
					if v.Kind() == reflect.Slice {
						fieldVal.Set(v)
					} else {
						fieldVal.Set(v.Elem())
					}
				}
			}
			return nil
		})
		for _, key := range keyList {
			if len(keyMap[key]) >= 2 {
				dfs = append(dfs, duplicateField{fieldName: key, values: keyMap[key]})
			}
		}

		return structVal.Addr()
	}

	doArrayEach = func(data []byte, bmsonType reflect.Type, fieldName string) reflect.Value {
		var sliceVal reflect.Value
		if bmsonType.Kind() == reflect.Slice {
			sliceVal = reflect.MakeSlice(bmsonType, 0, 0)
		} else if bmsonType.Kind() == reflect.Ptr && bmsonType.Elem().Kind() == reflect.Slice {
			sliceVal = reflect.MakeSlice(bmsonType.Elem(), 0, 0)
		} else {
			return reflect.Value{}
		}

		index := 0
		jsonparser.ArrayEach(data, func(value []byte, dataType jsonparser.ValueType, offset int, err error) {
			key := fmt.Sprintf("%s[%d]", fieldName, index)
			sliceVal = reflect.Append(sliceVal, getValue(value, key, dataType, bmsonType.Elem()).Elem())
			index++
		})
		return sliceVal
	}

	_, dataType, _, _ := jsonparser.Get(bytes, []string{}...)
	b := getValue(bytes, "root", dataType, reflect.TypeOf(_bmson))
	if !b.IsValid() {
		return nil, nil, nil, nil, fmt.Errorf("type of input json object is not bmson")
	}
	_bmson = b.Interface().(*_Bmson)

	// jsonと逆順になっているので反転させる
	for i, j := 0, len(dfs)-1; i < j; i, j = i+1, j-1 {
		dfs[i], dfs[j] = dfs[j], dfs[i]
	}

	return _bmson, ifs, its, dfs, nil
}

type requiredNilValue struct {
	fieldName string
}

func (v requiredNilValue) Log() Log {
	return Log{
		Level:      Error,
		Message:    fmt.Sprintf("%s is required, but is none", v.fieldName),
		Message_ja: fmt.Sprintf("%sは必須ですが、値がありません", v.fieldName),
	}
}

func convertBmson(_bmson *_Bmson) (bmsonData *bmson.Bmson, rns []requiredNilValue) {
	requiredButNil := func(fieldName string) {
		rns = append(rns, requiredNilValue{fieldName: fieldName})
	}

	var convert func(sourceValue reflect.Value, targetType reflect.Type, fieldName string) reflect.Value
	convert = func(sourceValue reflect.Value, targetType reflect.Type, fieldName string) reflect.Value {
		if !sourceValue.IsValid() {
			return reflect.Value{}
		}

		if sourceValue.Kind() == reflect.Ptr {
			if sourceValue.IsNil() {
				return reflect.Value{}
			} else {
				sourceValue = sourceValue.Elem()
			}
		}
		if targetType.Kind() == reflect.Ptr {
			targetType = targetType.Elem()
		}

		switch sourceValue.Kind() {
		case reflect.Struct:
			structVal := reflect.New(targetType).Elem()
			for i := 0; i < sourceValue.NumField(); i++ {
				fieldValue := sourceValue.Field(i)
				fieldType := targetType.Field(i)
				fullFieldName := fieldName + "." + strings.ToLower(fieldType.Name)
				if fieldValue.IsValid() {
					cv := convert(fieldValue, targetType.Field(i).Type, fullFieldName)
					if cv.IsValid() {
						if structVal.Field(i).Kind() == reflect.Ptr || structVal.Field(i).Kind() == reflect.Slice {
							structVal.Field(i).Set(cv)
						} else {
							structVal.Field(i).Set(cv.Elem())
						}

						if cv.Kind() == reflect.Slice && cv.Len() == 0 &&
							sourceValue.Type().Field(i).Tag.Get("validate") == "required" {
							requiredButNil(fullFieldName)
						}
					} else {
						// nil値
						if sourceValue.Type().Field(i).Tag.Get("validate") == "required" {
							requiredButNil(fullFieldName)
						}
					}
				} else {
					// 無効値?基本通ることは無い?
				}
			}
			return structVal.Addr()
		case reflect.Slice:
			sliceVal := reflect.MakeSlice(targetType, 0, 0)
			for i := 0; i < sourceValue.Len(); i++ {
				cv := convert(sourceValue.Index(i), targetType.Elem(), fmt.Sprintf("%s[%d]", fieldName, i))
				if targetType.Elem().Kind() == reflect.Ptr || targetType.Elem().Kind() == reflect.Slice {
					sliceVal = reflect.Append(sliceVal, cv)
				} else {
					sliceVal = reflect.Append(sliceVal, cv.Elem())
				}
			}
			return sliceVal
		}
		return sourceValue.Addr()
	}

	cv := convert(reflect.ValueOf(_bmson), reflect.TypeOf(bmson.Bmson{}), "root")
	bmsonData = cv.Interface().(*bmson.Bmson)
	if bmsonData.Info == nil {
		bmsonData.Info = &bmson.BmsonInfo{
			Mode_hint: "beat-7k",
		}
	}
	return bmsonData, rns
}

func ScanBmson(bytes []byte) (bmsonData *bmson.Bmson, logs Logs, _ error) {
	invalidBmsonFormatLog := func(err error) {
		logs = append(logs, Log{
			Level:      Error,
			Message:    fmt.Sprintf("Invalid bmson format: %s", err.Error()),
			Message_ja: fmt.Sprintf("bmsonのフォーマットが無効です: %s", err.Error()),
		})
	}

	// jsonフォーマットチェック
	var bmsonObj interface{}
	if err := json.Unmarshal(bytes, &bmsonObj); err != nil {
		invalidBmsonFormatLog(err)
		return nil, logs, err
	}

	// bytesから*_Bmsonを読み込む
	// 空の値はnilが入る
	validateBmson, ifs, its, dfs, err := unmarshalBmson(bytes)
	if err != nil {
		invalidBmsonFormatLog(err)
		return nil, logs, err
	}
	logs.addResultLog(ifs)
	logs.addResultLog(its)
	logs.addResultLog(dfs)

	// *_Bmsonを*bmsonに変換する
	// 変換前の値がnilの場合はゼロ値が入る
	bmsonData, rns := convertBmson(validateBmson)
	logs.addResultLog(rns)

	return bmsonData, logs, nil
}

func CheckBmsonFile(bmsonFile *BmsonFile) {
	if bmsonFile.IsInvalid {
		return
	}

	bmsonFile.Logs.addResultLogs(CheckBmsonInfo(bmsonFile))
	bmsonFile.Logs.addResultLogs(CheckTitleTextsAreDuplicate(bmsonFile))
	bmsonFile.Logs.addResultLogs(CheckSoundChannelNameIsInvalid(bmsonFile))
	bmsonFile.Logs.addResultLogs(CheckNonNotesSoundChannel(bmsonFile))
	bmsonFile.Logs.addResultLogs(CheckNoWavSoundChannels(bmsonFile))
	bmsonFile.Logs.addResultLogs(CheckTotalnotesIsZero(&bmsonFile.BmsFileBase))
	bmsonFile.Logs.addResultLogs(CheckSoundNotesIn0thMeasure(bmsonFile))
	bmsonFile.Logs.addResultLogs(CheckPlacedUndefiedBgaIds(bmsonFile))
	bmsonFile.Logs.addResultLogs(CheckDefinedUnplacedBgaHeader(bmsonFile))
	bmsonFile.Logs.addResultLogs(CheckBgaHeaderIdIsDuplicate(bmsonFile))
	bmsonFile.Logs.addResultLogs(CheckDuplicateY(bmsonFile))
	bmsonFile.Logs.addResultLogs(CheckNoteInLNBmson(bmsonFile))
	bmsonFile.Logs.addResultLogs(CheckWithoutKeysoundBmson(bmsonFile, nil))
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

type nonNotesSoundChannel struct {
	name  string
	index int
}

func (n nonNotesSoundChannel) Log() Log {
	return Log{
		Level:      Warning,
		Message:    fmt.Sprintf("sound_channels[%d].notes is empty: name:%s", n.index, n.name),
		Message_ja: fmt.Sprintf("sound_channels[%d].notesが空です: name:%s", n.index, n.name),
	}
}

func CheckNonNotesSoundChannel(bmsonFile *BmsonFile) (nss []nonNotesSoundChannel) {
	for i, soundChannel := range bmsonFile.Sound_channels {
		if len(soundChannel.Notes) == 0 {
			nss = append(nss, nonNotesSoundChannel{name: soundChannel.Name, index: i})
		}
	}
	return nss
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
	fileName     string
	channelIndex int
	note         bmson.Note
	noteIndex    int
}

func (n soundNote) string() string {
	return fmt.Sprintf("sound_channels[%d](%s)[%d] {x:%v, y:%d}", n.channelIndex, n.fileName, n.noteIndex, n.note.X, n.note.Y)
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
		log.SubLogs = append(log.SubLogs, soundNote.string())
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
	for ci, soundChannel := range bmsonFile.Sound_channels {
		for ni, note := range soundChannel.Notes {
			if x, ok := note.X.(float64); ok && x > 0 && note.Y < firstBarY {
				detectedSoundNotes = append(detectedSoundNotes, soundNote{
					fileName: soundChannel.Name, channelIndex: ci, note: note, noteIndex: ni})
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

type indexedHeader struct {
	index int
	bmson.BGAHeader
}

type definedUnplacedBgaHeader struct {
	headers []indexedHeader
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
		log.SubLogs = append(log.SubLogs, fmt.Sprintf("[%d] {id:%d name:%s}", header.index, header.Id, header.Name))
	}
	return log
}

func CheckDefinedUnplacedBgaHeader(bmsonFile *BmsonFile) (du *definedUnplacedBgaHeader) {
	if bmsonFile.Bga == nil {
		return nil
	}

	unplacedBgaHeaders := []indexedHeader{}
	bgaEventss := [][]bmson.BGAEvent{bmsonFile.Bga.Bga_events, bmsonFile.Bga.Layer_events, bmsonFile.Bga.Poor_events}
	for i, header := range bmsonFile.Bga.Bga_header {
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
			unplacedBgaHeaders = append(unplacedBgaHeaders, indexedHeader{index: i, BGAHeader: header})
		}
	}
	if len(unplacedBgaHeaders) > 0 {
		du = &definedUnplacedBgaHeader{headers: unplacedBgaHeaders}
	}
	return du
}

type duplicateBgaHeaderId struct {
	id             int
	indexedHeaders []indexedHeader
}

func (d duplicateBgaHeaderId) Log() Log {
	log := Log{
		Level:      Warning,
		Message:    fmt.Sprintf("bga_header has duplicate id: %d * %d", d.id, len(d.indexedHeaders)),
		Message_ja: fmt.Sprintf("bga_headerでidが重複しています: %d * %d", d.id, len(d.indexedHeaders)),
		SubLogs:    []string{},
		SubLogType: Detail,
	}
	for _, header := range d.indexedHeaders {
		log.SubLogs = append(log.SubLogs, fmt.Sprintf("bga_header[%d] {id:%d name:%s}", header.index, header.Id, header.Name))
	}
	return log
}

func CheckBgaHeaderIdIsDuplicate(bmsonFile *BmsonFile) (dis []duplicateBgaHeaderId) {
	if bmsonFile.Bga == nil {
		return nil
	}

	idMap := map[int][]indexedHeader{}
	for i, header := range bmsonFile.Bga.Bga_header {
		idMap[header.Id] = append(idMap[header.Id], indexedHeader{index: i, BGAHeader: header})
	}
	for id, headers := range idMap {
		if len(headers) >= 2 {
			dis = append(dis, duplicateBgaHeaderId{id: id, indexedHeaders: headers})
		}
	}
	sort.SliceStable(dis, func(i, j int) bool { return dis[i].id < dis[j].id })

	return dis
}

type yObject struct {
	y     int
	index int
	value interface{}
}

type yDuplicate struct {
	yValue    int
	fieldName string
	yObjects  []yObject
}

func (d yDuplicate) Log() Log {
	log := Log{
		Level:      Warning,
		Message:    fmt.Sprintf("%s has duplicate y values: %d * %d", d.fieldName, d.yValue, len(d.yObjects)),
		Message_ja: fmt.Sprintf("%sでy値が重複しています: %d * %d", d.fieldName, d.yValue, len(d.yObjects)),
		SubLogs:    []string{},
		SubLogType: Detail,
	}
	for _, obj := range d.yObjects {
		val := strings.ToLower(fmt.Sprintf("%+v", obj.value)) // フィールド名を小文字にする
		log.SubLogs = append(log.SubLogs, fmt.Sprintf("%s[%d] %+v", d.fieldName, obj.index, val))
	}
	return log
}

func CheckDuplicateY(bmsonFile *BmsonFile) (yds []yDuplicate) {
	checkDuplicateY := func(yObjects []yObject, fieldName string) {
		if len(yObjects) <= 1 {
			return
		}
		sort.SliceStable(yObjects, func(i, j int) bool { return yObjects[i].y < yObjects[j].y })
		sameYObjects := []yObject{yObjects[0]}
		for i := 1; i < len(yObjects); i++ {
			if yObjects[i-1].y == yObjects[i].y {
				sameYObjects = append(sameYObjects, yObjects[i])
			} else {
				if len(sameYObjects) >= 2 {
					tmpSlice := append([]yObject{}, sameYObjects...)
					yds = append(yds, yDuplicate{yValue: yObjects[i-1].y, fieldName: fieldName, yObjects: tmpSlice})
				}
				sameYObjects = []yObject{yObjects[i]}
			}
		}
		if len(sameYObjects) >= 2 {
			tmpSlice := append([]yObject{}, sameYObjects...)
			yds = append(yds, yDuplicate{yValue: yObjects[len(yObjects)-1].y, fieldName: fieldName, yObjects: tmpSlice})
		}
	}

	func() {
		yObjects := []yObject{}
		for i, line := range bmsonFile.Lines {
			yObjects = append(yObjects, yObject{y: line.Y, index: i, value: line})
		}
		checkDuplicateY(yObjects, "lines")
	}()

	for si, soundChannel := range bmsonFile.Sound_channels {
		yObjects := []yObject{}
		for i, note := range soundChannel.Notes {
			yObjects = append(yObjects, yObject{y: note.Y, index: i, value: note})
		}
		fieldName := fmt.Sprintf("sound_channels[%d](%s)", si, soundChannel.Name)
		checkDuplicateY(yObjects, fieldName)
	}

	func() {
		yObjects := []yObject{}
		for i, event := range bmsonFile.Bpm_events {
			yObjects = append(yObjects, yObject{y: event.Y, index: i, value: event})
		}
		checkDuplicateY(yObjects, "bpm_events")
	}()

	func() {
		yObjects := []yObject{}
		for i, event := range bmsonFile.Stop_events {
			yObjects = append(yObjects, yObject{y: event.Y, index: i, value: event})
		}
		checkDuplicateY(yObjects, "stop_events")
	}()

	func() {
		yObjects := []yObject{}
		for i, event := range bmsonFile.Scroll_events {
			yObjects = append(yObjects, yObject{y: event.Y, index: i, value: event})
		}
		checkDuplicateY(yObjects, "scroll_events")
	}()

	if bmsonFile.Bga != nil {
		func() {
			yObjects := []yObject{}
			for i, event := range bmsonFile.Bga.Bga_events {
				yObjects = append(yObjects, yObject{y: event.Y, index: i, value: event})
			}
			checkDuplicateY(yObjects, "bga_events")
		}()

		func() {
			yObjects := []yObject{}
			for i, event := range bmsonFile.Bga.Layer_events {
				yObjects = append(yObjects, yObject{y: event.Y, index: i, value: event})
			}
			checkDuplicateY(yObjects, "layer_events")
		}()

		func() {
			yObjects := []yObject{}
			for i, event := range bmsonFile.Bga.Poor_events {
				yObjects = append(yObjects, yObject{y: event.Y, index: i, value: event})
			}
			checkDuplicateY(yObjects, "poor_events")
		}()
	}

	return yds
}

type noteInLNBmson struct {
	containedNote *soundNote
	ln            *soundNote
}

func (n noteInLNBmson) Log() Log {
	noteType := "Normal"
	noteType_ja := "通常"
	if n.containedNote.note.L > 0 {
		noteType = "Long"
		noteType_ja = "ロング"
	}
	return Log{
		Level:      Error,
		Message:    fmt.Sprintf("%s note is in LN: %s in %s", noteType, n.containedNote.string(), n.ln.string()),
		Message_ja: fmt.Sprintf("%sノーツがLNの中に配置されています: %s in %s", noteType_ja, n.containedNote.string(), n.ln.string()),
	}
}

func CheckNoteInLNBmson(bmsonFile *BmsonFile) (nls []noteInLNBmson) {
	laneNumMap := map[int]int{5: 8, 7: 8, 9: 9, 10: 16, 14: 16, 24: 26, 48: 54}
	laneNum := laneNumMap[bmsonFile.Keymode]
	soundNotes := make([][]soundNote, laneNum)
	for ci, soundChannel := range bmsonFile.Sound_channels {
		for ni, note := range soundChannel.Notes {
			if x, ok := note.X.(float64); ok && x-math.Floor(x) == 0 && x > 0 /*&& int(x) <= laneNum*/ { // TODO xのチェック不要？
				xIndex := int(x) - 1
				soundNotes[xIndex] = append(soundNotes[xIndex], soundNote{
					fileName: soundChannel.Name, channelIndex: ci, note: note, noteIndex: ni})
			}
		}
	}

	for _, laneNotes := range soundNotes {
		sort.SliceStable(laneNotes, func(i, j int) bool { return laneNotes[i].note.Y < laneNotes[j].note.Y })
		var onGoingLN *soundNote
		for i := range laneNotes {
			if onGoingLN != nil {
				// LN開始地点はレイヤーノーツを考慮して範囲から外す。
				if laneNotes[i].note.Y > onGoingLN.note.Y && laneNotes[i].note.Y <= onGoingLN.note.Y+onGoingLN.note.L {
					// LN終了地点は終端音(up=true)を除外する
					if !(laneNotes[i].note.Y == onGoingLN.note.Y+onGoingLN.note.L && laneNotes[i].note.Up) {
						nls = append(nls, noteInLNBmson{containedNote: &laneNotes[i], ln: onGoingLN})
					}
				} else if onGoingLN.note.Y+onGoingLN.note.L < laneNotes[i].note.Y {
					onGoingLN = nil
				}
			}
			if laneNotes[i].note.L > 0 {
				onGoingLN = &laneNotes[i]
			}
		}
	}
	return nls
}

type withoutKeysoundMomentsBmson struct {
	withoutKeysoundMoments
}

type withoutKeysoundNotesBmson struct {
	wavFileIsExist  bool
	noWavNotes      []soundNote
	totalNotesCount int
}

func (wn withoutKeysoundNotesBmson) Log() Log {
	audioText := ""
	audioText_ja := ""
	if wn.wavFileIsExist {
		audioText = " (or audio file)"
		audioText_ja = "(または音声ファイル)"
	}
	notesText := fmt.Sprintf("%.1f%%(%d/%d)",
		float64(len(wn.noWavNotes))/float64(wn.totalNotesCount)*100, len(wn.noWavNotes), wn.totalNotesCount)
	return Log{
		Level:      Notice,
		Message:    fmt.Sprintf("Notes without keysound%s exist: %s", audioText, notesText),
		Message_ja: fmt.Sprintf("キー音%sの無いノーツがあります: %s", audioText_ja, notesText),
	}
}

func CheckWithoutKeysoundBmson(bmsonFile *BmsonFile, wavFileIsExist func(string) bool) (wm *withoutKeysoundMomentsBmson, wn *withoutKeysoundNotesBmson) {
	iss := CheckSoundChannelNameIsInvalid(bmsonFile)
	nonexistentWavIsPlaced := false
	isNoWavName := func(name string) bool {
		for _, invalidName := range iss {
			if name == invalidName.name {
				return true
			}
		}
		if wavFileIsExist != nil && !wavFileIsExist(name) {
			nonexistentWavIsPlaced = true
			return true
		}
		return false
	}

	soundNotes := []soundNote{}
	for _, soundChannel := range bmsonFile.Sound_channels {
		for _, note := range soundChannel.Notes {
			if x, ok := note.X.(float64); ok && x-math.Floor(x) == 0 && x > 0 {
				soundNotes = append(soundNotes, soundNote{fileName: soundChannel.Name, note: note})
			}
		}
	}
	sort.Slice(soundNotes, func(i, j int) bool { return soundNotes[i].note.Y < soundNotes[j].note.Y })

	noWavNotes := []soundNote{}
	noWavMoments := []int{}
	var momentY int
	momentIsNoWav := true
	totalMomentCount := 0
	for i, sNote := range soundNotes {
		if i == 0 {
			momentY = sNote.note.Y
			totalMomentCount++
		} else if momentY < sNote.note.Y {
			if momentIsNoWav {
				noWavMoments = append(noWavMoments, momentY)
			}
			momentY = sNote.note.Y
			momentIsNoWav = true
			totalMomentCount++
		}
		if isNoWavName(sNote.fileName) {
			noWavNotes = append(noWavNotes, sNote)
		} else {
			momentIsNoWav = false
		}
	}

	if wavFileIsExist != nil && !nonexistentWavIsPlaced {
		return nil, nil
	}

	if len(noWavMoments) > 0 {
		wm = &withoutKeysoundMomentsBmson{withoutKeysoundMoments: withoutKeysoundMoments{
			wavFileIsExist: wavFileIsExist != nil, noWavMomentCount: len(noWavMoments), momentCount: totalMomentCount}}
	}
	if len(noWavNotes) > 0 {
		wn = &withoutKeysoundNotesBmson{wavFileIsExist: wavFileIsExist != nil, noWavNotes: noWavNotes, totalNotesCount: len(soundNotes)}
	}

	return wm, wn
}

// 小数点以下の無駄な0を消去して整える
func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}
