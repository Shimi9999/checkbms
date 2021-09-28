package checkbms

import (
	"fmt"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	"github.com/Shimi9999/checkbms/audio"
	"github.com/Shimi9999/checkbms/diff"
)

func withoutExtPath(path string) string {
	return path[:len(path)-len(filepath.Ext(path))]
}

func relativePathFromBmsRoot(dirPath, path string) string {
	relativePath := filepath.Clean(path)
	rootDirPath := filepath.Clean(dirPath)
	if rootDirPath != "." {
		relativePath = filepath.Clean(relativePath[len(rootDirPath)+1:])
	}
	return relativePath
}

// pathのファイルがbmsDir.NonBmsFilesに含まれているかを返す。ついでにNonBmsFileのUsedをonにする。
func containsInNonBmsFiles(bmsDir *Directory, path string, exts []string, isBmson bool) bool {
	contains := false // 拡張子補完の対称ファイルを全てUsedにする
	definedFilePath := filepath.Clean(strings.ToLower(path))
	for i := range bmsDir.NonBmsFiles {
		//realFilePath := relativePathFromBmsRoot(bmsDir.Path, relativeToLower(bmsDir.Path, bmsDir.NonBmsFiles[i].Path))
		realFilePath := strings.ToLower(relativePathFromBmsRoot(bmsDir.Path, bmsDir.NonBmsFiles[i].Path))
		if definedFilePath == realFilePath {
			if isBmson {
				bmsDir.NonBmsFiles[i].Used_bmson = true
			} else {
				bmsDir.NonBmsFiles[i].Used_bms = true
			}
			contains = true
		} else if exts != nil && hasExts(realFilePath, exts) &&
			withoutExtPath(definedFilePath) == withoutExtPath(realFilePath) {
			if isBmson {
				bmsDir.NonBmsFiles[i].Used_bmson = true
			} else {
				bmsDir.NonBmsFiles[i].Used_bms = true
			}
			contains = true
		}
	}
	return contains
}

func isPreview(dirPath, path string) bool {
	for _, ext := range AUDIO_EXTS {
		if regexp.MustCompile(`^preview.*` + ext + `$`).MatchString(strings.ToLower(relativePathFromBmsRoot(dirPath, path))) {
			return true
		}
	}
	return false
}

type notExistFile struct {
	level    AlertLevel
	dirPath  string
	bmsPath  string
	filePath string
	command  string
}

func notExistFileLog(nf notExistFile, isBmson bool) Log {
	label := "#" + strings.ToUpper(nf.command)
	if isBmson {
		label = nf.command
	}
	return Log{
		Level:      nf.level,
		Message:    fmt.Sprintf("Defined file does not exist(%s): %s %s", relativePathFromBmsRoot(nf.dirPath, nf.bmsPath), label, nf.filePath),
		Message_ja: fmt.Sprintf("定義されているファイルが実在しません(%s): %s %s", relativePathFromBmsRoot(nf.dirPath, nf.bmsPath), label, nf.filePath),
	}
}

func (nf notExistFile) Log() Log {
	return notExistFileLog(nf, false)
}

func CheckDefinedFilesExist(bmsDir *Directory, bmsFile *BmsFile) (nfs []notExistFile) {
	check := func(commands []string, exts []string) {
		for _, command := range commands {
			val, ok := bmsFile.Header[command]
			if ok && val != "" {
				if !containsInNonBmsFiles(bmsDir, val, exts, false) {
					nfs = append(nfs, notExistFile{level: Warning, dirPath: bmsDir.Path, bmsPath: bmsFile.Path, filePath: val, command: command})
				}
			}
		}
	}
	imageCommands := []string{"stagefile", "banner", "backbmp"}
	check(imageCommands, nil) // 画像は拡張子補完が行われないプレイヤーもあるので、補完を考慮しない
	audioCommands := []string{"preview"}
	check(audioCommands, AUDIO_EXTS)

	return nfs
}

//pathsOfdoNotExistWavs := []string{}
func CheckDefinedWavFilesExist(bmsDir *Directory, bmsFile *BmsFile) (nfs []notExistFile) {
	for _, def := range bmsFile.HeaderWav {
		if def.Value != "" {
			if !containsInNonBmsFiles(bmsDir, def.Value, AUDIO_EXTS, false) {
				nfs = append(nfs, notExistFile{level: Error, dirPath: bmsDir.Path, bmsPath: bmsFile.Path, filePath: def.Value, command: def.command()})
			}
		}
	}
	return nfs
}

func CheckDefinedBmpFilesExist(bmsDir *Directory, bmsFile *BmsFile) (nfs []notExistFile) {
	for _, def := range bmsFile.HeaderBmp {
		if def.Value != "" {
			exts := IMAGE_EXTS
			if hasExts(def.Value, MOVIE_EXTS) {
				exts = append(MOVIE_EXTS, IMAGE_EXTS...)
			}
			if !containsInNonBmsFiles(bmsDir, def.Value, exts, false) {
				nfs = append(nfs, notExistFile{level: Error, dirPath: bmsDir.Path, bmsPath: bmsFile.Path, filePath: def.Value, command: def.command()})
			}
		}
	}
	return nfs
}

type notExistFileBmson struct {
	notExistFile
}

func (nf notExistFileBmson) Log() Log {
	return notExistFileLog(nf.notExistFile, true)
}

type definedPath struct {
	path       string
	fieldName  string
	exts       []string
	alertLevel AlertLevel
}

func checkDefinedPathsExistBmson(bmsDir *Directory, bmsonFile *BmsonFile, definedPaths []definedPath) (nfs []notExistFileBmson) {
	for _, defiedPath := range definedPaths {
		if defiedPath.path != "" && !containsInNonBmsFiles(bmsDir, defiedPath.path, defiedPath.exts, true) {
			nfs = append(nfs, notExistFileBmson{notExistFile: notExistFile{
				level: defiedPath.alertLevel, dirPath: bmsDir.Path, bmsPath: bmsonFile.Path, filePath: defiedPath.path, command: defiedPath.fieldName}})
		}
	}
	return nfs
}

func CheckDefinedInfoFilesExistBmson(bmsDir *Directory, bmsonFile *BmsonFile) (nfs []notExistFileBmson) {
	defiedPaths := []definedPath{
		{path: bmsonFile.Info.Back_image, fieldName: "info.back_image", exts: nil, alertLevel: Warning},
		{path: bmsonFile.Info.Eyecatch_image, fieldName: "info.eyecatch_image", exts: nil, alertLevel: Warning},
		{path: bmsonFile.Info.Title_image, fieldName: "info.title_image", exts: nil, alertLevel: Warning},
		{path: bmsonFile.Info.Banner_image, fieldName: "info.banner_image", exts: nil, alertLevel: Warning},
		{path: bmsonFile.Info.Preview_music, fieldName: "info.preview_music", exts: AUDIO_EXTS, alertLevel: Warning},
	}
	return checkDefinedPathsExistBmson(bmsDir, bmsonFile, defiedPaths)
}

func CheckDefinedSoundFilesExistBmson(bmsDir *Directory, bmsonFile *BmsonFile) (nfs []notExistFileBmson) {
	defiedPaths := []definedPath{}
	for i, soundChannel := range bmsonFile.Sound_channels {
		defiedPaths = append(defiedPaths, definedPath{
			path:       soundChannel.Name,
			fieldName:  fmt.Sprintf("sound_channel[%d]", i),
			exts:       AUDIO_EXTS,
			alertLevel: Error})
	}
	return checkDefinedPathsExistBmson(bmsDir, bmsonFile, defiedPaths)
}

func CheckDefinedBgaFilesExistBmson(bmsDir *Directory, bmsonFile *BmsonFile) (nfs []notExistFileBmson) {
	defiedPaths := []definedPath{}
	if bmsonFile.Bga != nil {
		for i, header := range bmsonFile.Bga.Bga_header {
			exts := IMAGE_EXTS
			if hasExts(header.Name, MOVIE_EXTS) {
				exts = append(MOVIE_EXTS, IMAGE_EXTS...)
			}
			defiedPaths = append(defiedPaths, definedPath{
				path:       header.Name,
				fieldName:  fmt.Sprintf("bga_header[%d](id:%d)", i, header.Id),
				exts:       exts,
				alertLevel: Error})
		}
	}
	return checkDefinedPathsExistBmson(bmsDir, bmsonFile, defiedPaths)
}

type notUnifiedDefinition struct {
	bmsFilePath string
	value       string
}

type notUnifiedDefinitions struct {
	command string
	defs    []notUnifiedDefinition
}

func (nd notUnifiedDefinitions) Log() Log {
	log := Log{
		Level:      Warning,
		Message:    fmt.Sprintf("%s are not unified", nd.command),
		Message_ja: fmt.Sprintf("%sが統一されていません", nd.command),
		SubLogs:    []string{},
		SubLogType: Detail,
	}
	for _, def := range nd.defs {
		log.SubLogs = append(log.SubLogs, fmt.Sprintf("%s: %s", def.bmsFilePath, def.value))
	}
	return log
}

func CheckDefinitionsAreUnified(bmsDir *Directory) (nds []notUnifiedDefinitions) {
	type unifyCommand struct {
		bmsCommand   string
		bmsonField   string
		isNotUnified bool
	}

	unifyCommands := []unifyCommand{
		{bmsCommand: "stagefile", bmsonField: "eyecatch_image"},
		{bmsCommand: "banner", bmsonField: "banner_image"},
		{bmsCommand: "backbmp", bmsonField: "title_image"},
		{bmsonField: "back_image"},
		{bmsCommand: "preview", bmsonField: "preview_music"},
	}

	valueAndPaths := make([][]notUnifiedDefinition, len(unifyCommands))
	for i, uc := range unifyCommands {
		if uc.bmsCommand != "" {
			for _, bmsFile := range bmsDir.BmsFiles {
				valueAndPaths[i] = append(valueAndPaths[i], notUnifiedDefinition{
					bmsFilePath: relativePathFromBmsRoot(bmsDir.Path, bmsFile.Path),
					value:       bmsFile.Header[uc.bmsCommand]})
				if len(valueAndPaths[i]) >= 2 && valueAndPaths[i][len(valueAndPaths[i])-2].value != bmsFile.Header[uc.bmsCommand] {
					unifyCommands[i].isNotUnified = true
				}
			}
		}
		for _, bmsonFile := range bmsDir.BmsonFiles {
			fieldName := strings.ToUpper(uc.bmsonField[:1]) + uc.bmsonField[1:]
			infoValue := reflect.ValueOf(bmsonFile.Info).Elem()
			if value := infoValue.FieldByName(fieldName); value.IsValid() {
				valStr := value.String()
				valueAndPaths[i] = append(valueAndPaths[i], notUnifiedDefinition{
					bmsFilePath: relativePathFromBmsRoot(bmsDir.Path, bmsonFile.Path),
					value:       valStr})
				if len(valueAndPaths[i]) >= 2 && valueAndPaths[i][len(valueAndPaths[i])-2].value != valStr {
					unifyCommands[i].isNotUnified = true
				}
			}
		}
	}

	for i, uc := range unifyCommands {
		if uc.isNotUnified {
			commandStr := ""
			if len(bmsDir.BmsFiles) > 0 && uc.bmsCommand != "" {
				commandStr += "#" + strings.ToUpper(uc.bmsCommand)
				if len(bmsDir.BmsonFiles) > 0 {
					commandStr += fmt.Sprintf("(info.%s)", uc.bmsonField)
				}
			} else if len(bmsDir.BmsonFiles) > 0 {
				commandStr += "info." + uc.bmsonField
			}
			nds = append(nds, notUnifiedDefinitions{command: commandStr, defs: valueAndPaths[i]})
		}
	}
	return nds
}

type unusedFile struct {
	path string
}

func (uf unusedFile) Log() Log {
	return Log{
		Level:      Notice,
		Message:    fmt.Sprintf("This file is not used: %s", uf.path),
		Message_ja: fmt.Sprintf("このファイルは使用されていません: %s", uf.path),
	}
}

func CheckUnusedFile(bmsDir *Directory) (ufs []unusedFile) {
	ignoreExts := []string{".txt", ".zip", ".rar", ".lzh", ".7z"}
	for _, nonBmsFile := range bmsDir.NonBmsFiles {
		if !nonBmsFile.UsedFromAny() && !hasExts(nonBmsFile.Path, ignoreExts) && !isPreview(bmsDir.Path, nonBmsFile.Path) {
			ufs = append(ufs, unusedFile{path: relativePathFromBmsRoot(bmsDir.Path, nonBmsFile.Path)})
		}
	}
	return ufs
}

type emptyDirectory struct {
	path string
}

func (ed emptyDirectory) Log() Log {
	return Log{
		Level:      Notice,
		Message:    fmt.Sprintf("This directory is empty: %s", ed.path),
		Message_ja: fmt.Sprintf("このフォルダは空です: %s", ed.path),
	}
}

func CheckEmptyDirectory(bmsDir *Directory) (eds []emptyDirectory) {
	for _, dir := range bmsDir.Directories {
		if len(dir.BmsFiles) == 0 && len(dir.NonBmsFiles) == 0 && len(dir.Directories) == 0 {
			eds = append(eds, emptyDirectory{path: relativePathFromBmsRoot(bmsDir.Path, dir.Path)})
		}
	}
	return eds
}

type environmentDependentFilename struct {
	path string
}

func (ef environmentDependentFilename) Log() Log {
	return Log{
		Level:      Warning,
		Message:    fmt.Sprintf("This filename has environment-dependent characters: %s", ef.path),
		Message_ja: fmt.Sprintf("このファイル名は環境依存文字を含んでいます: %s", ef.path),
	}
}

// must do after used check
func CheckEnvironmentDependentFilename(bmsDir *Directory) (efs []environmentDependentFilename) {
	for _, file := range bmsDir.BmsFiles {
		if rPath := relativePathFromBmsRoot(bmsDir.Path, file.Path); containsMultibyteRune(rPath) {
			efs = append(efs, environmentDependentFilename{path: rPath})
		}
	}
	for _, file := range bmsDir.NonBmsFiles {
		if rPath := relativePathFromBmsRoot(bmsDir.Path, file.Path); (file.UsedFromAny() || strings.ToLower(filepath.Ext(file.Path)) == ".txt" || isPreview(bmsDir.Path, file.Path)) && containsMultibyteRune(rPath) {
			efs = append(efs, environmentDependentFilename{path: rPath})
		}
	}
	return efs
}

type over1MinuteAudioFile struct {
	duration float64
	path     string
}

func (oa over1MinuteAudioFile) Log() Log {
	return Log{
		Level:      Warning,
		Message:    fmt.Sprintf("This audio file is over 1 minute(%.1fsec): %s", oa.duration, oa.path),
		Message_ja: fmt.Sprintf("この音声ファイルは1分以上あります(%.1fsec): %s", oa.duration, oa.path),
	}
}

// must do after CheckDefinedWavFilesExist
// TODO 引数としてUsedリストを受け取る？
func CheckOver1MinuteAudioFile(bmsDir *Directory) (oas []over1MinuteAudioFile) {
	for _, file := range bmsDir.NonBmsFiles {
		if file.Used_bms && hasExts(file.Path, AUDIO_EXTS) {
			if d, _ := audio.Duration(file.Path); d >= 60.0 {
				oas = append(oas, over1MinuteAudioFile{duration: d, path: relativePathFromBmsRoot(bmsDir.Path, file.Path)})
			}
		}
	}
	return oas
}

type sameHashBmsFiles struct {
	paths []string
}

func (sb sameHashBmsFiles) Log() Log {
	log := Log{
		Level:      Warning,
		Message:    "These bmsfiles are same",
		Message_ja: "これらのBMSファイルは同一です",
		SubLogs:    []string{},
		SubLogType: Detail,
	}
	log.SubLogs = append(log.SubLogs, sb.paths...)
	return log
}

func CheckSameHashBmsFiles(bmsDir *Directory) (sbs []sameHashBmsFiles) {
	tmpBmsFiles := []BmsFileBase{}
	for _, bmsFile := range bmsDir.BmsFiles {
		tmpBmsFiles = append(tmpBmsFiles, bmsFile.BmsFileBase)
	}
	for _, bmsonFile := range bmsDir.BmsonFiles {
		tmpBmsFiles = append(tmpBmsFiles, bmsonFile.BmsFileBase)
	}
	for i := 0; i < len(tmpBmsFiles); i++ {
		samePaths := []string{tmpBmsFiles[i].Path}
		for j := i + 1; j < len(tmpBmsFiles); j++ {
			if tmpBmsFiles[i].Sha256 == tmpBmsFiles[j].Sha256 {
				samePaths = append(samePaths, tmpBmsFiles[j].Path)
				if j+1 < len(tmpBmsFiles) { // 同ハッシュの組み合わせが重複しないように、一度該当した要素は削除する
					tmpBmsFiles = append(tmpBmsFiles[:j], tmpBmsFiles[j+1:]...)
				}
			}
		}
		if len(samePaths) > 1 {
			sbs = append(sbs, sameHashBmsFiles{paths: samePaths})
		}
	}
	return sbs
}

type pathAndStrings struct {
	path string
	strs []string
}

func groupsStrings(groups [][]string) (strs []string) {
	for i, group := range groups {
		groupStr := ""
		for j, str := range group {
			groupStr += str
			if j < len(group)-1 {
				groupStr += ", "
			}
		}
		strs = append(strs, fmt.Sprintf("Group%d: %s", i+1, groupStr))
	}
	return strs
}

func groupStringSlice(pathAndStringss []pathAndStrings) (groups [][]string) {
	pss := append([]pathAndStrings{}, pathAndStringss...)
	for len(pss) > 0 {
		targetStructre := pss[0]
		groups = append(groups, []string{targetStructre.path})
		gi := len(groups) - 1
		pss = pss[1:]
		for j := 0; j < len(pss); j++ {
			if reflect.DeepEqual(targetStructre.strs, pss[j].strs) {
				groups[gi] = append(groups[gi], pss[j].path)
				pss = append(pss[:j], pss[j+1:]...)
				j--
			}
		}
	}
	return groups
}

type notUnifiedIndexedDefinition struct {
	otype      objType
	pathGroups [][]string
}

func (ni notUnifiedIndexedDefinition) Log() Log {
	return Log{
		Level:      Warning,
		Message:    fmt.Sprintf("#%sxx are not unified", strings.ToUpper(ni.otype.string())),
		Message_ja: fmt.Sprintf("#%sxxが統一されていません", strings.ToUpper(ni.otype.string())),
		SubLogs:    groupsStrings(ni.pathGroups),
		SubLogType: Detail,
	}
}

func CheckIndexedDefinitionsAreUnified(bmsDir *Directory) (nis []notUnifiedIndexedDefinition) {
	otypes := []objType{Bmp, Wav}
	for _, otype := range otypes {
		makeDefStrs := func(defs []indexedDefinition) (defStrs []string) {
			for _, def := range defs {
				defStrs = append(defStrs, fmt.Sprintf("#%s %s", strings.ToUpper(def.command()), def.Value))
			}
			return defStrs
		}
		definitions := []pathAndStrings{}
		for i := range bmsDir.BmsFiles {
			definitions = append(definitions, pathAndStrings{
				path: relativePathFromBmsRoot(bmsDir.Path, bmsDir.BmsFiles[i].Path),
				strs: makeDefStrs(bmsDir.BmsFiles[i].headerIndexedDefs(otype))})
		}

		groups := groupStringSlice(definitions)
		if len(groups) > 1 {
			nis = append(nis, notUnifiedIndexedDefinition{otype: otype, pathGroups: groups})
		}
	}
	return nis
}

type notUnifiedObjectStructure struct {
	otype      objType
	pathGroups [][]string
}

func (no notUnifiedObjectStructure) Log() Log {
	return Log{
		Level:      Warning,
		Message:    fmt.Sprintf("%s object structures are not unified", strings.ToUpper(no.otype.string())),
		Message_ja: fmt.Sprintf("%sオブジェ構成が統一されていません", strings.ToUpper(no.otype.string())),
		SubLogs:    groupsStrings(no.pathGroups),
		SubLogType: Detail,
	}
}

func CheckObjectStructuresAreUnified(bmsDir *Directory) (nos []notUnifiedObjectStructure) {
	otypes := []objType{Bmp, Wav}
	for _, otype := range otypes {
		makeObjStrs := func(bmsFile *BmsFile) (objStrs []string) {
			for _, obj := range bmsFile.bmsObjs(otype) {
				if !obj.IsLNEnd && bmsFile.definedValue(otype, obj.value36()) != "" {
					pos := obj.Position
					pos.reduce()
					objStrs = append(objStrs, fmt.Sprintf("%d-%d/%d %s", obj.Measure, pos.Numerator, pos.Denominator, obj.value36()))
				}
			}
			return objStrs
		}
		structures := []pathAndStrings{}
		for i := range bmsDir.BmsFiles {
			structures = append(structures, pathAndStrings{
				path: relativePathFromBmsRoot(bmsDir.Path, bmsDir.BmsFiles[i].Path),
				strs: makeObjStrs(&bmsDir.BmsFiles[i])})
		}

		groups := groupStringSlice(structures)
		if len(groups) > 1 {
			nos = append(nos, notUnifiedObjectStructure{otype: otype, pathGroups: groups})
		}
	}
	return nos
}

type definitionDiff struct {
	oType       objType
	pathI       string
	pathJ       string
	missingDefs []missingDef
}

type missingDef struct {
	path  string
	value string
}

func (dd definitionDiff) Log() Log {
	log := Log{
		Level:      Warning,
		Message:    fmt.Sprintf("There are %d differences in %s definitions: %s %s", len(dd.missingDefs), dd.oType.string(), dd.pathI, dd.pathJ),
		Message_ja: fmt.Sprintf("%s定義に%d個の違いがあります: %s %s", dd.oType.string(), len(dd.missingDefs), dd.pathI, dd.pathJ),
		SubLogs:    []string{},
		SubLogType: Detail,
	}
	for _, mDef := range dd.missingDefs {
		log.SubLogs = append(log.SubLogs, fmt.Sprintf("Missing(%s): %s", mDef.path, mDef.value)) // TODO 日本語対応
	}
	return log
}

func CheckDefinitionDiff(bmsDirPath string, bmsFileI, bmsFileJ *BmsFile) (dds []definitionDiff) {
	if bmsFileI.Sha256 == bmsFileJ.Sha256 {
		return nil
	}
	diffDefs := func(t objType, iBmsFile, jBmsFile *BmsFile) {
		iDefs, jDefs := iBmsFile.headerIndexedDefs(t), jBmsFile.headerIndexedDefs(t)
		iDefStrs, jDefStrs := []string{}, []string{}
		for _, def := range iDefs {
			iDefStrs = append(iDefStrs, fmt.Sprintf("#%s %s", strings.ToUpper(def.command()), def.Value))
		}
		for _, def := range jDefs {
			jDefStrs = append(jDefStrs, fmt.Sprintf("#%s %s", strings.ToUpper(def.command()), def.Value))
		}
		ed, ses := diff.Onp(iDefStrs, jDefStrs)
		if ed > 0 {
			missingDefs := []missingDef{}
			i, j := 0, 0
			for _, r := range ses {
				switch r {
				case '=':
					i++
					j++
				case '+':
					missingDefs = append(missingDefs, missingDef{path: relativePathFromBmsRoot(bmsDirPath, iBmsFile.Path), value: jDefStrs[j]})
					j++
				case '-':
					missingDefs = append(missingDefs, missingDef{path: relativePathFromBmsRoot(bmsDirPath, jBmsFile.Path), value: iDefStrs[i]})
					i++
				}
			}
			dds = append(dds, definitionDiff{oType: t,
				pathI: relativePathFromBmsRoot(bmsDirPath, iBmsFile.Path), pathJ: relativePathFromBmsRoot(bmsDirPath, jBmsFile.Path),
				missingDefs: missingDefs})
		}
	}
	diffDefs(Wav, bmsFileI, bmsFileJ)
	diffDefs(Bmp, bmsFileI, bmsFileJ)
	return dds
}

type objectDiff struct {
	oType       objType
	pathI       string
	pathJ       string
	missingObjs []missingObj
}

type missingObj struct {
	path  string
	value string
}

func (od objectDiff) Log() Log {
	log := Log{
		Level:      Warning,
		Message:    fmt.Sprintf("There are %d differences in %s objects: %s, %s", len(od.missingObjs), od.oType.string(), od.pathI, od.pathJ),
		Message_ja: fmt.Sprintf("%sオブジェに%d個の違いがあります: %s %s", od.oType.string(), len(od.missingObjs), od.pathI, od.pathJ),
		SubLogs:    []string{},
		SubLogType: Detail,
	}
	for _, mObj := range od.missingObjs {
		log.SubLogs = append(log.SubLogs, fmt.Sprintf("Missing(%s): %s", mObj.path, mObj.value)) // TODO 日本語対応
	}
	return log
}

func CheckObjectDiff(bmsDirPath string, bmsFileI, bmsFileJ *BmsFile) (ods []objectDiff) {
	if bmsFileI.Sha256 == bmsFileJ.Sha256 {
		return nil
	}
	diffObjs := func(t objType, iBmsFile, jBmsFile *BmsFile) {
		missingObjs := []missingObj{}
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
					missingObjs = append(missingObjs, missingObj{path: relativePathFromBmsRoot(bmsDirPath, jBmsFile.Path), value: iObj.string(iBmsFile)})
				}
				ii++
			} else {
				if jBmsFile.definedValue(t, jObj.value36()) != "" {
					missingObjs = append(missingObjs, missingObj{path: relativePathFromBmsRoot(bmsDirPath, iBmsFile.Path), value: jObj.string(jBmsFile)})
				}
				jj++
			}
		}
		for ; ii < len(iObjs); ii++ {
			iObj := iObjs[ii]
			if !iObj.IsLNEnd && iBmsFile.definedValue(t, iObj.value36()) != "" {
				missingObjs = append(missingObjs, missingObj{path: relativePathFromBmsRoot(bmsDirPath, jBmsFile.Path), value: iObj.string(iBmsFile)})
			}
		}
		for ; jj < len(jObjs); jj++ {
			jObj := jObjs[jj]
			if !jObj.IsLNEnd && jBmsFile.definedValue(t, jObj.value36()) != "" {
				missingObjs = append(missingObjs, missingObj{path: relativePathFromBmsRoot(bmsDirPath, iBmsFile.Path), value: jObj.string(jBmsFile)})
			}
		}
		if len(missingObjs) > 0 {
			ods = append(ods, objectDiff{oType: t,
				pathI: relativePathFromBmsRoot(bmsDirPath, iBmsFile.Path), pathJ: relativePathFromBmsRoot(bmsDirPath, jBmsFile.Path),
				missingObjs: missingObjs})
		}
	}
	diffObjs(Wav, bmsFileI, bmsFileJ)
	diffObjs(Bmp, bmsFileI, bmsFileI)
	return ods
}
