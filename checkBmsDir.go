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

func relativeToLower(dirPath, path string) string { // TODO なぜToLowerしている？この関数の意味は？
	relative := strings.ToLower(relativePathFromBmsRoot(dirPath, path))
	return path[:len(path)-len(relative)] + relative
}

// pathのファイルがbmsDir.NonBmsFilesに含まれているかを返す。ついでにNonBmsFileのUsedをonにする。
func containsInNonBmsFiles(bmsDir *Directory, path string, exts []string) bool {
	contains := false // 拡張子補完の対称ファイルを全てUsedにする
	definedFilePath := filepath.Clean(strings.ToLower(path))
	for i := range bmsDir.NonBmsFiles {
		//realFilePath := relativePathFromBmsRoot(bmsDir.Path, relativeToLower(bmsDir.Path, bmsDir.NonBmsFiles[i].Path))
		realFilePath := strings.ToLower(relativePathFromBmsRoot(bmsDir.Path, bmsDir.NonBmsFiles[i].Path))
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

func (nf notExistFile) Log() Log {
	return Log{
		Level:      nf.level,
		Message:    fmt.Sprintf("Defined file does not exist(%s): #%s %s", relativePathFromBmsRoot(nf.dirPath, nf.bmsPath), strings.ToUpper(nf.command), nf.filePath),
		Message_ja: fmt.Sprintf("定義されているファイルが実在しません(%s): #%s %s", relativePathFromBmsRoot(nf.dirPath, nf.bmsPath), strings.ToUpper(nf.command), nf.filePath),
	}
}

func CheckDefinedFilesExist(bmsDir *Directory, bmsFile *BmsFile) (nfs []notExistFile) {
	check := func(commands []string, exts []string) {
		for _, command := range commands {
			val, ok := bmsFile.Header[command]
			if ok && val != "" {
				if !containsInNonBmsFiles(bmsDir, val, exts) {
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
			if !containsInNonBmsFiles(bmsDir, def.Value, AUDIO_EXTS) {
				nfs = append(nfs, notExistFile{level: Error, dirPath: bmsDir.Path, bmsPath: bmsFile.Path, filePath: def.Value, command: def.command()})
			}
		}
	}
	return nfs
}

func CheckDefinedBpmFilesExist(bmsDir *Directory, bmsFile *BmsFile) (nfs []notExistFile) {
	for _, def := range bmsFile.HeaderBmp {
		if def.Value != "" {
			exts := IMAGE_EXTS
			if hasExts(def.Value, MOVIE_EXTS) {
				exts = append(MOVIE_EXTS, IMAGE_EXTS...)
			}
			if !containsInNonBmsFiles(bmsDir, def.Value, exts) {
				nfs = append(nfs, notExistFile{level: Error, dirPath: bmsDir.Path, bmsPath: bmsFile.Path, filePath: def.Value, command: def.command()})
			}
		}
	}
	return nfs
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
		Message:    fmt.Sprintf("#%s is not unified", strings.ToUpper(nd.command)),
		Message_ja: fmt.Sprintf("#%sが統一されていません", strings.ToUpper(nd.command)),
		SubLogs:    []string{},
		SubLogType: Detail,
	}
	for _, def := range nd.defs {
		log.SubLogs = append(log.SubLogs, fmt.Sprintf("%s: %s", def.bmsFilePath, def.value))
	}
	return log
}

func CheckDefinitionsAreUnified(bmsDir *Directory) (nds []notUnifiedDefinitions) {
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
			defs := []notUnifiedDefinition{}
			for j, bmsFile := range bmsDir.BmsFiles {
				defs = append(defs, notUnifiedDefinition{bmsFilePath: relativePathFromBmsRoot(bmsDir.Path, bmsFile.Path), value: values[index][j]})
			}
			nds = append(nds, notUnifiedDefinitions{command: uc, defs: defs})
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
		if !nonBmsFile.Used && !hasExts(nonBmsFile.Path, ignoreExts) && !isPreview(bmsDir.Path, nonBmsFile.Path) {
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
		if rPath := relativePathFromBmsRoot(bmsDir.Path, file.Path); (file.Used || strings.ToLower(filepath.Ext(file.Path)) == ".txt" || isPreview(bmsDir.Path, file.Path)) && containsMultibyteRune(rPath) {
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
		if file.Used && hasExts(file.Path, AUDIO_EXTS) {
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
	tmpBmsFiles := []BmsFile{}
	copy(tmpBmsFiles, bmsDir.BmsFiles)
	for i := 0; i < len(tmpBmsFiles); i++ {
		samePaths := []string{bmsDir.BmsFiles[i].Path}
		for j := i + 1; j < len(tmpBmsFiles); j++ {
			if bmsDir.BmsFiles[i].Sha256 == bmsDir.BmsFiles[j].Sha256 {
				samePaths = append(samePaths, bmsDir.BmsFiles[j].Path)
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

type notUnifiedIndexedDefinition struct {
	otype      objType
	pathGroups [][]string
}

func (ni notUnifiedIndexedDefinition) Log() Log {
	log := Log{
		Level:      Warning,
		Message:    fmt.Sprintf("#%sxx is not unified", strings.ToUpper(ni.otype.string())),
		Message_ja: fmt.Sprintf("#%sxxが統一されていません", strings.ToUpper(ni.otype.string())),
		SubLogs:    []string{},
		SubLogType: Detail,
	}
	for i, pathGroup := range ni.pathGroups {
		groupStr := ""
		for j, bmsFilePath := range pathGroup {
			groupStr += bmsFilePath
			if j < len(pathGroup)-1 {
				groupStr += ", "
			}
		}
		log.SubLogs = append(log.SubLogs, fmt.Sprintf("Group%d: %s", i+1, groupStr))
	}
	return log
}

func CheckIndexedDefinitionAreUnified(bmsDir *Directory) (nis []notUnifiedIndexedDefinition) {
	type definition struct {
		bmsFilePath string
		defStrs     []string
	}
	otypes := []objType{Bmp, Wav}
	for _, otype := range otypes {
		makeDefStrs := func(defs []indexedDefinition) (defStrs []string) {
			for _, def := range defs {
				defStrs = append(defStrs, fmt.Sprintf("#%s %s", strings.ToUpper(def.command()), def.Value))
			}
			return defStrs
		}
		definitions := []definition{}
		for i := range bmsDir.BmsFiles {
			definitions = append(definitions, definition{
				bmsFilePath: relativePathFromBmsRoot(bmsDir.Path, bmsDir.BmsFiles[i].Path),
				defStrs:     makeDefStrs(bmsDir.BmsFiles[i].headerIndexedDefs(otype))})
		}

		groups := [][]string{}
		for len(definitions) > 0 {
			targetDefinition := definitions[0]
			groups = append(groups, []string{targetDefinition.bmsFilePath})
			gi := len(groups) - 1
			definitions = definitions[1:]
			for j := 0; j < len(definitions); j++ {
				if reflect.DeepEqual(targetDefinition.defStrs, definitions[j].defStrs) {
					groups[gi] = append(groups[gi], definitions[j].bmsFilePath)
					definitions = append(definitions[:j], definitions[j+1:]...)
					j--
				}
			}
		}

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
	log := Log{
		Level:      Warning,
		Message:    fmt.Sprintf("%s object structure is not unified", strings.ToUpper(no.otype.string())),
		Message_ja: fmt.Sprintf("%sオブジェ構成が統一されていません", strings.ToUpper(no.otype.string())),
		SubLogs:    []string{},
		SubLogType: Detail,
	}
	for i, pathGroup := range no.pathGroups {
		groupStr := ""
		for j, bmsFilePath := range pathGroup {
			groupStr += bmsFilePath
			if j < len(pathGroup)-1 {
				groupStr += ", "
			}
		}
		log.SubLogs = append(log.SubLogs, fmt.Sprintf("Group%d: %s", i+1, groupStr))
	}
	return log
}

func CheckObjectStructreIsUnified(bmsDir *Directory) (nos []notUnifiedObjectStructure) {
	type structure struct {
		bmsFilePath string
		objStrs     []string
	}
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
		structures := []structure{}
		for i := range bmsDir.BmsFiles {
			structures = append(structures, structure{
				bmsFilePath: relativePathFromBmsRoot(bmsDir.Path, bmsDir.BmsFiles[i].Path),
				objStrs:     makeObjStrs(&bmsDir.BmsFiles[i])})
		}

		groups := [][]string{}
		for len(structures) > 0 {
			targetStructre := structures[0]
			groups = append(groups, []string{targetStructre.bmsFilePath})
			gi := len(groups) - 1
			structures = structures[1:]
			for j := 0; j < len(structures); j++ {
				if reflect.DeepEqual(targetStructre.objStrs, structures[j].objStrs) {
					groups[gi] = append(groups[gi], structures[j].bmsFilePath)
					structures = append(structures[:j], structures[j+1:]...)
					j--
				}
			}
		}

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
