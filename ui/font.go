// Package ui — текст и 2D-примитивы поверх сцены.
//
// Путь до пикселя:
//
//	Font (TTF/OTF)  →  Atlas (A8, покрытие глифов)  →  Draw (батч квадов)
//	                →  Renderer (пайплайн Vulkan)   →  кадр
//
// В Vulkan нет функций для текста: шрифт растеризуется на CPU в альбом
// одноканальной картинки (atlas), а на экран попадает обычными квадами
// с UV-координатами в этом атласе (см. internal/shaders/quad.vert).
package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
)

// Font — загруженный TTF/OTF: разбор один, кэш face'ей по размерам.
type Font struct {
	// Path — откуда загружен (для логов и UI).
	Path string

	src   *opentype.Font
	faces map[int]font.Face
}

// ParseFont разбирает шрифт из памяти.
func ParseFont(data []byte, name string) (*Font, error) {
	f, err := opentype.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("разбор шрифта %s: %w", name, err)
	}
	return &Font{Path: name, src: f, faces: map[int]font.Face{}}, nil
}

// LoadFont читает шрифт с диска.
func LoadFont(path string) (*Font, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("чтение шрифта %s: %w", path, err)
	}
	return ParseFont(data, path)
}

// Face возвращает face для размера в пикселях (кешируется).
//
// Размер задаётся в пикселях при DPI 72, поэтому 1 pt == 1 px и текст
// ложится ровно в пиксельную сетку кадра.
func (f *Font) Face(px int) (font.Face, error) {
	if face, ok := f.faces[px]; ok {
		return face, nil
	}
	face, err := opentype.NewFace(f.src, &opentype.FaceOptions{
		Size:    float64(px),
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return nil, fmt.Errorf("шрифт %s, размер %d: %w", f.Path, px, err)
	}
	f.faces[px] = face
	return face, nil
}

// Measure возвращает ширину строки в пикселях для размера px.
func (f *Font) Measure(s string, px int) float32 {
	face, err := f.Face(px)
	if err != nil {
		return 0
	}
	return float32(font.MeasureString(face, s)) / 64
}

// Ascent — высота над базовой линией (px).
func (f *Font) Ascent(px int) float32 {
	face, err := f.Face(px)
	if err != nil {
		return 0
	}
	return float32(face.Metrics().Ascent) / 64
}

// Descent — глубина под базовой линией (px, положительное число).
func (f *Font) Descent(px int) float32 {
	face, err := f.Face(px)
	if err != nil {
		return 0
	}
	return float32(face.Metrics().Descent) / 64
}

// DefaultFontPaths — кандидаты системных шрифтов, когда путь не задан.
func DefaultFontPaths() []string {
	var out []string
	switch runtime.GOOS {
	case "windows":
		root := os.Getenv("WINDIR")
		if root == "" {
			root = `C:\Windows`
		}
		for _, n := range []string{
			"segoeui.ttf", "segoeuib.ttf", "tahoma.ttf", "arial.ttf", "verdana.ttf",
			"consola.ttf", "cour.ttf",
		} {
			out = append(out, filepath.Join(root, "Fonts", n))
		}
	case "darwin":
		out = append(out,
			"/System/Library/Fonts/Supplemental/Arial.ttf",
			"/System/Library/Fonts/Helvetica.ttc",
			"/Library/Fonts/Arial Unicode.ttf",
		)
	default:
		for _, n := range []string{
			"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
			"/usr/share/fonts/truetype/liberation/LiberationSans-Regular.ttf",
			"/usr/share/fonts/truetype/noto/NotoSans-Regular.ttf",
			"/usr/share/fonts/TTF/DejaVuSans.ttf",
			"/usr/share/fonts/dejavu/DejaVuSans.ttf",
			"/usr/share/fonts/truetype/freefont/FreeSans.ttf",
		} {
			out = append(out, n)
		}
	}
	return out
}

// FindFont выбирает файл шрифта: явный путь, затем шрифты проекта
// (assets/fonts/*.ttf, *.otf), затем системные гарнитуры.
func FindFont(explicit string) (string, error) {
	if explicit != "" {
		if _, err := os.Stat(explicit); err != nil {
			return "", fmt.Errorf("шрифт %q не найден: %w", explicit, err)
		}
		return explicit, nil
	}

	var cands []string
	for _, pat := range []string{"assets/fonts/*.ttf", "assets/fonts/*.otf", "assets/*.ttf"} {
		if matches, err := filepath.Glob(pat); err == nil {
			cands = append(cands, matches...)
		}
	}
	cands = append(cands, DefaultFontPaths()...)

	for _, p := range cands {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
	}
	return "", fmt.Errorf("шрифт не найден; укажите -font путь.ttf (проверено: %s)",
		strings.Join(cands, ", "))
}
