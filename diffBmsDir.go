package checkbms

import (
	"fmt"
	"path/filepath"
	"sort"
)

// 2つのディレクトリ内のファイル分布が一致するか比較して確認する。比較はファイルパスで行う。
// 音声、画像、動画ファイルを比較する時は、パスの拡張子を取り除き、名前だけで比較する。
// bmsfileは同名ファイルでsha256の比較をする。一致しなかった場合はファイル記述内容のdiffをとる。
func DiffBmsDirectories(dirPath1, dirPath2 string) (logs []string, _ error) {
	const dirlen int = 2
	dirPaths := [dirlen]string{dirPath1, dirPath2}
	for _, dirPath := range dirPaths {
		if !IsBmsDirectory(dirPath) {
			return nil, fmt.Errorf("This is not BMS Directory path: %s", dirPath)
		}
	}

	bmsDirs := [dirlen]Directory{}
	for i, dirPath := range dirPaths {
		bmsDir, err := scanBmsDirectory(dirPath, true, false)
		if err != nil {
			return nil, err
		}
		bmsDirs[i] = *bmsDir
	}

	type compareFile struct {
		File
		ComparePath string
		BmsFileData *BmsFile
	}
	type compareDirectory struct {
		BmsFiles    []compareFile
		AudioFiles  []compareFile
		ImageFiles  []compareFile
		MovieFiles  []compareFile
		OtherFiles  []compareFile
		Directories []compareFile
	}

	comDirs := [dirlen]compareDirectory{}
	removeRootDirPath := func(path, rootdirpath string) string {
		return path[len(rootdirpath)+1:]
	}
	for i := 0; i < dirlen; i++ {
		for j, bmsFile := range bmsDirs[i].BmsFiles {
			bmsFile.Path = removeRootDirPath(bmsFile.Path, dirPaths[i])
			comDirs[i].BmsFiles = append(comDirs[i].BmsFiles, compareFile{File: bmsFile.File, ComparePath: bmsFile.Path, BmsFileData: &bmsDirs[i].BmsFiles[j]})
		}
		for _, dir := range bmsDirs[i].Directories {
			dir.Path = removeRootDirPath(dir.Path, dirPaths[i])
			comDirs[i].Directories = append(comDirs[i].Directories, compareFile{File: dir.File, ComparePath: dir.Path})
		}
		for _, nonBmsFile := range bmsDirs[i].NonBmsFiles {
			nonBmsFile.Path = removeRootDirPath(nonBmsFile.Path, dirPaths[i])
			completedPath := nonBmsFile.Path[:len(nonBmsFile.Path)-len(filepath.Ext(nonBmsFile.Path))]
			if hasExts(nonBmsFile.Path, AUDIO_EXTS) {
				comDirs[i].AudioFiles = append(comDirs[i].AudioFiles, compareFile{File: nonBmsFile.File, ComparePath: completedPath})
			} else if hasExts(nonBmsFile.Path, IMAGE_EXTS) {
				comDirs[i].ImageFiles = append(comDirs[i].ImageFiles, compareFile{File: nonBmsFile.File, ComparePath: completedPath})
			} else if hasExts(nonBmsFile.Path, MOVIE_EXTS) {
				comDirs[i].MovieFiles = append(comDirs[i].MovieFiles, compareFile{File: nonBmsFile.File, ComparePath: completedPath})
			} else {
				comDirs[i].OtherFiles = append(comDirs[i].OtherFiles, compareFile{File: nonBmsFile.File, ComparePath: nonBmsFile.Path})
			}
		}
	}

	sortSliceWithPath := func(cf []compareFile) []compareFile {
		sort.Slice(cf, func(i, j int) bool { return cf[i].ComparePath < cf[j].ComparePath })
		return cf
	}
	for i := 0; i < dirlen; i++ {
		comDirs[i].BmsFiles = sortSliceWithPath(comDirs[i].BmsFiles)
		comDirs[i].AudioFiles = sortSliceWithPath(comDirs[i].AudioFiles)
		comDirs[i].ImageFiles = sortSliceWithPath(comDirs[i].ImageFiles)
		comDirs[i].MovieFiles = sortSliceWithPath(comDirs[i].MovieFiles)
		comDirs[i].OtherFiles = sortSliceWithPath(comDirs[i].OtherFiles)
		comDirs[i].Directories = sortSliceWithPath(comDirs[i].Directories)
	}

	hashLogs, missingLogs1, missingLogs2 := []string{}, []string{}, []string{}

	missingLog := func(dirPath, missingPath string) string {
		return fmt.Sprintf("%s is missing the file: %s", dirPath, missingPath)
	}
	comFileSlices1 := []([]compareFile){comDirs[0].BmsFiles, comDirs[0].AudioFiles, comDirs[0].ImageFiles, comDirs[0].MovieFiles, comDirs[0].OtherFiles, comDirs[0].Directories}
	comFileSlices2 := []([]compareFile){comDirs[1].BmsFiles, comDirs[1].AudioFiles, comDirs[1].ImageFiles, comDirs[1].MovieFiles, comDirs[1].OtherFiles, comDirs[1].Directories}
	for i := 0; i < 6; i++ {
		comFiles1, comFiles2 := comFileSlices1[i], comFileSlices2[i]
		i2init := 0
		for i1 := 0; i1 < len(comFiles1); i1++ {
			if i2init == len(comFiles2) {
				missingLogs2 = append(missingLogs2, missingLog(dirPath2, comFiles1[i1].Path))
				continue
			}
			for i2 := i2init; i2 < len(comFiles2); i2++ {
				if comFiles1[i1].ComparePath == comFiles2[i2].ComparePath {
					if comFiles1[i1].BmsFileData != nil && comFiles2[i2].BmsFileData != nil {
						if comFiles1[i1].BmsFileData.Sha256 != comFiles2[i2].BmsFileData.Sha256 {
							// ここでファイル内容diff
							hashLogs = append(hashLogs, fmt.Sprintf("%s: Each hash(sha256) is not equal:\n  %s: %s\n  %s: %s",
								comFiles1[i1].Path, dirPath1, comFiles1[i1].BmsFileData.Sha256, dirPath2, comFiles2[i2].BmsFileData.Sha256))
						}
					}
					i2init = i2 + 1
					break
				} else {
					if comFiles1[i1].ComparePath < comFiles2[i2].ComparePath {
						missingLogs2 = append(missingLogs2, missingLog(dirPath2, comFiles1[i1].Path))
						break
					} else {
						missingLogs1 = append(missingLogs1, missingLog(dirPath1, comFiles2[i2].Path))
					}
				}
			}
		}
		for i2 := i2init; i2 < len(comFiles2); i2++ {
			missingLogs1 = append(missingLogs1, missingLog(dirPath1, comFiles2[i2].Path))
		}
	}

	logs = append(hashLogs, append(missingLogs1, missingLogs2...)...)

	return logs, nil
}
