package diff

import (
  "strings"
)

/*func main() {
  strs1 := []string{"aaa", "bbb", "ccc", "ddd", "eee", "f", "g", "h"}
  strs2 := []string{"aaa", "bbb", "ccc", "d"}
  fmt.Println(strs1, strs2)

  ed, ses := Onp(&strs1, &strs2)
  fmt.Println("Edit distance:", ed)
  println("SES:", ses)
  lcs := ""
  i1, i2 := 0, 0
  for _, r := range ses {
    switch r {
    case '=':
      lcs += strs2[i1] + ","
      i1++
      i2++
    case '+':
      fmt.Println("+", strs2[i2])
      i2++
    case '-':
      fmt.Println("-", strs1[i1])
      i1++
    }
  }
  println("LCS:", lcs)
}*/

func Onp(strsM, strsN []string) (int, string) {
  isReverse := false
  if len(strsM) > len(strsN) {
    tmp := strsM
    strsM = strsN
    strsN = tmp
    isReverse = true
  }
  m, n := len(strsM), len(strsN)
  delta := n - m

  snake := func(k, y int) int {
    x := y - k
    for x < m && y < n && strsM[x] == strsN[y] {
      x++
      y++
    }
    return y
  }

  max := func(x, y int) int {
    if x > y {
      return x
    }
    return y
  }

  offset := m + 1
  fp := make([]int, m+n+3)
  fpRoot := make([]string, m+n+3)
  for i := range fp {
    fp[i] = -1
  }
  root := func(p, k int) {
    isOrigin := k == 0 && fp[k-1+offset] == -1 && fp[k-1+offset] == -1
    if fp[k-1+offset]+1 > fp[k+1+offset] { // n < m で>=にする？(+-の順番を統一するため)
      fpRoot[k+offset] = fpRoot[k-1+offset]
      if !isOrigin {
        if !isReverse {
          fpRoot[k+offset] += "+"
        } else {
          fpRoot[k+offset] += "-"
        }
      }
    } else {
      fpRoot[k+offset] = fpRoot[k+1+offset]
      if !isOrigin {
        if !isReverse {
          fpRoot[k+offset] += "-"
        } else {
          fpRoot[k+offset] += "+"
        }
      }
    }
    fpRoot[k+offset] += strings.Repeat("=", fp[k+offset] - max(fp[k-1+offset]+1, fp[k+1+offset]))
  }
  for p := 0; ; p++ {
    for k := -p; k <= delta-1; k++ {
      fp[k+offset] = snake(k, max(fp[k-1+offset]+1, fp[k+1+offset]))
      root(p, k)
    }
    for k := delta + p; k >= delta+1; k-- {
      fp[k+offset] = snake(k, max(fp[k-1+offset]+1, fp[k+1+offset]))
      root(p, k)
    }
    fp[delta+offset] = snake(delta, max(fp[delta-1+offset]+1, fp[delta+1+offset]))
    root(p, delta)
    if fp[delta+offset] == n {
      return delta + 2*p, fpRoot[delta+offset]
    }
  }
  return -1, ""
}
