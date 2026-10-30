package main

import (
	"bytes"
	"context"
	"embed"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/BH2VSQ/SATSSTV-Go/internal/sstv"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed all:frontend
var assets embed.FS

type App struct{ ctx context.Context }

func (a *App) startup(ctx context.Context) { a.ctx = ctx }

type Mode struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	Ready       bool   `json:"ready"`
	Description string `json:"description"`
}

func (a *App) GetModes() []Mode {
	out := make([]Mode, 0)
	for _, m := range sstv.Modes() {
		out = append(out, Mode{m.ID, m.Name, m.Width, m.Height, m.CodecReady, m.Description})
	}
	return out
}

func (a *App) SelectImage() string {
	p, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title:   "选择 SSTV 发送图片",
		Filters: []runtime.FileFilter{{DisplayName: "图片文件", Pattern: "*.png;*.jpg;*.jpeg"}},
	})
	if err != nil {
		return ""
	}
	return p
}

// LoadImagePreview returns a data URL so the WebView can display a file selected
// through the native file dialog without granting it arbitrary filesystem access.
func (a *App) LoadImagePreview(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("图片路径为空")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if len(data) == 0 {
		return "", fmt.Errorf("图片文件为空")
	}
	_, _, err = image.Decode(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("读取图片失败：%w", err)
	}
	mime := "image/png"
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg":
		mime = "image/jpeg"
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}

// EncodeImage remains available for callers that encode a source file directly.
func (a *App) EncodeImage(input, mode string, rate int) (string, error) {
	if strings.TrimSpace(input) == "" {
		return "", fmt.Errorf("请先选择图片")
	}
	f, err := os.Open(input)
	if err != nil {
		return "", err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return "", fmt.Errorf("读取图片失败：%w", err)
	}
	return a.encodeAndSave(img, filepath.Base(input), mode, rate)
}

// EncodeImageData encodes the final image composed in the GUI canvas. This is
// what makes editable callsign/QTH/message overlays part of the transmitted SSTV frame.
func (a *App) EncodeImageData(dataURL, fileName, mode string, rate int) (string, error) {
	if strings.TrimSpace(dataURL) == "" {
		return "", fmt.Errorf("发送画面为空")
	}
	parts := strings.SplitN(dataURL, ",", 2)
	if len(parts) != 2 || !strings.Contains(parts[0], ";base64") {
		return "", fmt.Errorf("发送画面数据格式无效")
	}
	data, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("读取发送画面失败：%w", err)
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("解析发送画面失败：%w", err)
	}
	if strings.TrimSpace(fileName) == "" {
		fileName = "SATSSTV-Go"
	}
	return a.encodeAndSave(img, fileName, mode, rate)
}

func (a *App) encodeAndSave(img image.Image, fileName, mode string, rate int) (string, error) {
	if rate < 8000 || rate > 192000 {
		return "", fmt.Errorf("采样率应为 8000 至 192000 Hz")
	}
	spec, err := sstv.LookupMode(mode)
	if err != nil {
		return "", err
	}
	if !spec.CodecReady {
		return "", fmt.Errorf("%s 的编码器尚未实现", spec.Name)
	}
	enc := sstv.NewEncoder(rate)
	samples, err := enc.Encode(a.ctx, img, mode)
	if err != nil {
		return "", err
	}
	base := strings.TrimSuffix(filepath.Base(fileName), filepath.Ext(fileName))
	if base == "" || base == "." {
		base = "SATSSTV-Go"
	}
	out, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "保存 SSTV 音频",
		DefaultFilename: base + "-" + mode + ".wav",
		Filters:         []runtime.FileFilter{{DisplayName: "WAV 音频", Pattern: "*.wav"}},
	})
	if err != nil {
		return "", err
	}
	if out == "" {
		return "", fmt.Errorf("已取消保存")
	}
	wf, err := os.Create(out)
	if err != nil {
		return "", err
	}
	defer wf.Close()
	if err := sstv.WriteWAV(wf, samples, rate); err != nil {
		return "", err
	}
	return out, nil
}

func main() {
	app := &App{}
	err := wails.Run(&options.App{
		Title: "SATSSTV-Go", Width: 1500, Height: 960, MinWidth: 1080, MinHeight: 720,
		AssetServer: &assetserver.Options{Assets: assets}, OnStartup: app.startup, Bind: []interface{}{app},
	})
	if err != nil {
		println("启动 GUI 失败:", err.Error())
	}
}
