package audio

import (
  "fmt"
  "os"
  //"log"
  "strings"
  "path/filepath"

  "github.com/faiface/beep"
  "github.com/faiface/beep/wav"
  "github.com/faiface/beep/vorbis"
  "github.com/faiface/beep/mp3"
  "github.com/faiface/beep/flac"
)

/*func main() {
  if len(os.Args) < 2 {
    fmt.Println("Usage: sound <soundpath>")
    os.Exit(1)
  }
  path := os.Args[1]

  d, err := Duration(path)
  if err != nil {
    log.Fatal(err)
  }
  fmt.Printf("%s duration: %fs\n", path, d)
}*/

func Duration(path string) (float64, error) {
  f, err := os.Open(path)
  if err != nil {
    return 0, err
  }
  defer f.Close()

  var stream beep.StreamSeekCloser
  var format beep.Format
  err = fmt.Errorf("not audio file: %s", path)
  switch strings.ToLower(filepath.Ext(path)) {
  case ".wav":
    stream, format, err = wav.Decode(f)
  case ".ogg":
    stream, format, err = vorbis.Decode(f)
  case ".mp3":
    stream, format, err = mp3.Decode(f)
  case ".flac":
    stream, format, err = flac.Decode(f)
  }
  if err == nil {
    defer stream.Close()
    return float64(stream.Len()) / float64(format.SampleRate), nil
  }
  return 0, err
}
