// Example program exercising github.com/malcolmston/chalk.
//
// It demonstrates: chained immutable styles, package-level shortcuts, the
// 256/truecolor/hex/HSL-HSV-HWB color models, level degradation via
// Style.Level, global level control + capability probes, nesting behaviour,
// multi-line handling, the ANSI-aware width utilities (used to build an aligned
// table of colored cells), and the color-space conversion helpers.
package main

import (
	"fmt"
	"strings"

	"github.com/malcolmston/chalk"
)

func section(title string) {
	fmt.Println()
	fmt.Println(chalk.New().Bold().Underline().Sprint("== " + title))
}

// showRaw prints the escape sequences literally so the demo is verifiable even
// when stdout is not a terminal.
func showRaw(label, s string) {
	fmt.Printf("  %-28s %s\n", label+":", strings.ReplaceAll(s, "\x1b", "\\x1b"))
}

func main() {
	// Force truecolor so output is deterministic when piped (otherwise chalk
	// auto-detects and emits nothing at all off-terminal).
	chalk.SetLevel(chalk.LevelTrueColor)

	fmt.Println(chalk.New().Bold().BgBlue().White().Sprint(" chalk example "))

	section("1. Chained styles (immutable, reusable)")
	base := chalk.New().Bold()
	danger := base.Red()
	notice := base.Cyan().Underline()
	// base itself is unchanged by the two derivations above.
	fmt.Println("  " + danger.Sprint("danger") + " / " + notice.Sprint("notice") + " / " + base.Sprint("plain bold"))
	showRaw("danger raw", danger.Sprint("danger"))
	showRaw("base still just bold", base.Sprint("x"))
	fmt.Println("  " + chalk.New().Italic().Strikethrough().Dim().Sprint("italic+strike+dim"))
	fmt.Println("  " + chalk.New().Inverse().Sprint("inverse") + " " + chalk.New().Overline().Sprint("overline"))
	fmt.Printf("  Sprintf: %s\n", chalk.New().Yellow().Sprintf("%d items in %s", 42, "queue"))
	_, _ = chalk.New().Magenta().Println("  Println via Style")

	section("2. Package-level shortcuts")
	fmt.Println("  " + chalk.Red("red") + " " + chalk.Green("green") + " " + chalk.Blue("blue") +
		" " + chalk.Gray("gray") + " " + chalk.BrightYellow("bright-yellow"))
	fmt.Println("  " + chalk.BgGreen(chalk.Black(" PASS ")) + " " + chalk.BgRed(chalk.BrightWhite(" FAIL ")))
	fmt.Println("  " + chalk.Hex("#ff8800", "hex orange") + " " + chalk.Ansi256(214, "ansi256 214") +
		" " + chalk.RGB(120, 200, 255, "rgb"))
	fmt.Println("  " + chalk.HSL(300, 100, 50, "hsl") + " " + chalk.HSV(180, 100, 100, "hsv") +
		" " + chalk.HWB(30, 0, 20, "hwb"))

	section("3. Level degradation of colors")
	levels := []struct {
		name string
		l    chalk.Level
	}{
		{"LevelTrueColor", chalk.LevelTrueColor},
		{"Level256", chalk.Level256},
		{"LevelBasic", chalk.LevelBasic},
		{"LevelNone", chalk.LevelNone},
	}
	// In v0.4.0 the concrete escape code for a color is chosen when the style is
	// BUILT, from whatever level is in force at that moment. So degradation has to
	// be driven by setting the level *before* constructing the style.
	fmt.Println("  built after Level(...) is applied (works):")
	for _, lv := range levels {
		showRaw(lv.name, chalk.New().Level(lv.l).Hex("#ff8800").Sprint("hot"))
	}
	showRaw("BgHex @Basic (built after)", chalk.New().Level(chalk.LevelBasic).BgHex("#00aa00").Sprint("bg"))

	// HOLE: chaining .Level() *after* a color does not re-resolve that color —
	// the escape sequence was already frozen. A style stored in a package variable
	// therefore cannot be re-targeted at a different terminal capability, and a
	// later chalk.SetLevel does not affect it either. Every line below prints the
	// same truecolor sequence, which is why it is only shown, not asserted.
	warn := chalk.New().Hex("#ff8800") // built while level == LevelTrueColor
	fmt.Println("  same style with Level(...) chained afterwards (does NOT degrade):")
	for _, lv := range levels {
		showRaw(lv.name, warn.Level(lv.l).Sprint("hot"))
	}
	fmt.Println("  ^ LevelNone still suppresses output, but 256/basic keep the truecolor code.")

	section("4. Global level + capability probes")
	fmt.Printf("  level=%d enabled=%v basic=%v 256=%v truecolor=%v\n",
		chalk.GetLevel(), chalk.Enabled(), chalk.HasBasic(), chalk.Has256(), chalk.HasTrueColor())
	chalk.SetEnabled(false)
	showRaw("color disabled", chalk.Red("no codes here"))
	fmt.Printf("  SupportsColor()=%v\n", chalk.SupportsColor())
	// Visible() suppresses output entirely while color is off.
	showRaw("Visible() while off", "["+chalk.Visible("hidden when uncolored")+"]")
	chalk.SetLevel(chalk.LevelTrueColor)
	showRaw("Visible() while on", "["+chalk.Visible("shown")+"]")
	chalk.ResetDetection()
	fmt.Printf("  after ResetDetection level=%d (auto-detected)\n", chalk.GetLevel())
	chalk.SetLevel(chalk.LevelTrueColor)

	section("5. Nesting and newlines")
	nested := chalk.New().Red().Sprint("red " + chalk.Green("green") + " red again")
	showRaw("nested re-opens outer", nested)
	showRaw("empty input", "["+chalk.Red("")+"]")
	showRaw("multi-line", chalk.Blue("line1\nline2"))
	showRaw("crlf", chalk.Blue("a\r\nb"))

	section("6. ANSI-aware measurement utilities")
	cell := chalk.Green("图表")
	fmt.Printf("  len(bytes)=%d VisibleLength(runes)=%d\n", len(cell), chalk.VisibleLength(cell))
	// HOLE: v0.4.0 has no VisibleWidth (terminal cells) and no RuneWidth, so
	// there is no way to measure the display width of CJK text or emoji — the
	// following would not compile:
	//   chalk.VisibleWidth(cell)   // undefined
	//   chalk.RuneWidth('图')       // undefined
	fmt.Println("  HOLE: no VisibleWidth/RuneWidth — only rune counts are available.")

	// HOLE: Strip is `\x1b\[[0-9;]*m` only. Non-SGR CSI sequences (\x1b[2J) and
	// OSC sequences (\x1b]0;title\x07) survive, so a "stripped" string can still
	// contain escapes and still mis-measure.
	fmt.Printf("  Strip(SGR only)     = %q\n", chalk.Strip("\x1b[31mred\x1b[39m"))
	fmt.Printf("  Strip(CSI+OSC left) = %q\n",
		strings.ReplaceAll(chalk.Strip("\x1b[2J\x1b[31mred\x1b[39m\x1b]0;title\x07done"), "\x1b", "\\x1b"))

	// Align a table using VisibleLength. Note the CJK row is visibly misaligned in
	// a real terminal: VisibleLength counts 图表 as 2, but it occupies 4 cells.
	rows := [][2]string{
		{chalk.Cyan("name"), chalk.Cyan("status")},
		{chalk.Bold("图表-service"), chalk.BgGreen(chalk.Black(" up "))},
		{chalk.Bold("auth"), chalk.BgRed(chalk.BrightWhite(" down "))},
		{chalk.Bold("cache"), chalk.Yellow("degraded")},
	}
	widths := [2]int{}
	for _, r := range rows {
		for i, c := range r {
			if w := chalk.VisibleLength(c); w > widths[i] {
				widths[i] = w
			}
		}
	}
	for _, r := range rows {
		fmt.Print("  ")
		for i, c := range r {
			fmt.Print(c, strings.Repeat(" ", widths[i]-chalk.VisibleLength(c)+2))
		}
		fmt.Println()
	}
	fmt.Println("  ^ the 图表 row is under-padded: rune count != cell width.")

	section("7. Color-space conversions")
	r, g, b := chalk.HexToRGB("#ff8800")
	fmt.Printf("  HexToRGB(#ff8800)   = %d,%d,%d\n", r, g, b)
	fmt.Printf("  RGBToHex(%d,%d,%d)  = %s\n", r, g, b, chalk.RGBToHex(r, g, b))
	fmt.Printf("  RGBToAnsi256        = %d\n", chalk.RGBToAnsi256(r, g, b))
	fmt.Printf("  RGBToAnsi16         = %d\n", chalk.RGBToAnsi16(r, g, b))
	fmt.Printf("  Ansi256ToAnsi16(214)= %d\n", chalk.Ansi256ToAnsi16(214))
	ar, ag, ab := chalk.Ansi256ToRGB(214)
	fmt.Printf("  Ansi256ToRGB(214)   = %d,%d,%d\n", ar, ag, ab)
	h, s, l := chalk.RGBToHSL(r, g, b)
	fmt.Printf("  RGBToHSL            = %d,%d,%d -> HSLToRGB roundtrip = %s\n",
		h, s, l, rgbTriple(chalk.HSLToRGB(h, s, l)))
	hh, ss, v := chalk.RGBToHSV(r, g, b)
	fmt.Printf("  RGBToHSV            = %d,%d,%d -> HSVToRGB roundtrip = %s\n",
		hh, ss, v, rgbTriple(chalk.HSVToRGB(hh, ss, v)))
	wh, ww, wb := chalk.RGBToHWB(r, g, b)
	fmt.Printf("  RGBToHWB            = %d,%d,%d -> HWBToRGB roundtrip = %s\n",
		wh, ww, wb, rgbTriple(chalk.HWBToRGB(wh, ww, wb)))

	section("8. Reset / Hidden shortcuts")
	showRaw("Reset()", chalk.Reset("plain"))
	showRaw("Hidden()", chalk.Hidden("secret"))

	fmt.Println()
	fmt.Println(chalk.New().BgBrightGreen().Black().Sprint(" done "))
}

func rgbTriple(r, g, b int) string { return fmt.Sprintf("%d,%d,%d", r, g, b) }
