package checkbms

import (
	"encoding/json"
	"fmt"
	"math"
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
		} else if keyName == "total" {
			bmsonTotal := fv.Float()
			total := CalculateDefaultTotal(bmsonFile.TotalNotes, bmsonFile.Keymode) * bmsonTotal / 100
			if total < 100 {
				logs = append(logs, Log{
					Level:      Warning,
					Message:    fmt.Sprintf("Real total value is under 100: %f", total),
					Message_ja: fmt.Sprintf("実際のTotal値が100未満です: %f", total),
				})
			} else if bmsonFile.TotalNotes > 0 {
				totalJudge := JudgeOverTotal(total, bmsonFile.TotalNotes, bmsonFile.Keymode)
				if totalJudge > 0 {
					logs = append(logs, Log{
						Level:      Notice,
						Message:    fmt.Sprintf("Real total value is very high(TotalNotes=%d): %f", bmsonFile.TotalNotes, total),
						Message_ja: fmt.Sprintf("実際のTotal値がかなり高いです(トータルノーツ=%d): %f", bmsonFile.TotalNotes, total),
					})
				} else if totalJudge < 0 {
					logs = append(logs, Log{
						Level:      Notice,
						Message:    fmt.Sprintf("Real total value is very low(TotalNotes=%d): %f", bmsonFile.TotalNotes, total),
						Message_ja: fmt.Sprintf("実際のTotal値がかなり低いです(トータルノーツ=%d): %f", bmsonFile.TotalNotes, total),
					})
				}
			}
		}
	}

	return logs
}
