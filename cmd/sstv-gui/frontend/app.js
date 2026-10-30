'use strict';

const $ = (id) => document.getElementById(id);
const defaultTemplate = 'SSTV QSO  |  {MYCALL} → {HISCALL}\nQTH: {QTH}   LOC: {LOCATOR}\nREPORT: {REPORT}\n{MESSAGE}\n{DATE}  {TIME}';
const fields = ['myCall', 'hisCall', 'qth', 'locator', 'report', 'message'];
let modes = [];
let selectedImagePath = '';
let sourceImage = null;

function setResult(text, error = false) {
  $('txResult').textContent = text;
  $('txResult').style.color = error ? '#f5a9b8' : '#a9dff2';
}

function localDateTime() {
  const d = new Date();
  const pad = (n) => String(n).padStart(2, '0');
  return {
    DATE: `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`,
    TIME: `${pad(d.getHours())}:${pad(d.getMinutes())}`,
  };
}

function resolvedTemplate() {
  const values = {
    MYCALL: $('myCall').value.trim(),
    HISCALL: $('hisCall').value.trim(),
    QTH: $('qth').value.trim(),
    LOCATOR: $('locator').value.trim(),
    REPORT: $('report').value.trim(),
    MESSAGE: $('message').value.trim(),
    ...localDateTime(),
  };
  return $('templateText').value.replace(/\{([A-Z]+)\}/g, (token, key) => {
    return Object.prototype.hasOwnProperty.call(values, key) && values[key] ? values[key] : token;
  });
}

function drawCover(ctx, img, width, height) {
  const scale = Math.max(width / img.width, height / img.height);
  const sw = width / scale;
  const sh = height / scale;
  const sx = (img.width - sw) / 2;
  const sy = (img.height - sh) / 2;
  ctx.drawImage(img, sx, sy, sw, sh, 0, 0, width, height);
}

function wrapText(ctx, text, maxWidth) {
  const output = [];
  const paragraphs = String(text).split('\n');
  for (const paragraph of paragraphs) {
    if (!paragraph) { output.push(''); continue; }
    let line = '';
    for (const char of paragraph) {
      const candidate = line + char;
      if (line && ctx.measureText(candidate).width > maxWidth) {
        output.push(line);
        line = char;
      } else {
        line = candidate;
      }
    }
    output.push(line);
  }
  return output;
}

function renderTxCanvas() {
  const canvas = $('txCanvas');
  const ctx = canvas.getContext('2d');
  const selectedMode = modes.find((m) => m.id === $('mode').value);
  const width = selectedMode?.width || 320;
  const height = selectedMode?.height || 240;
  canvas.width = Math.max(160, width);
  canvas.height = Math.max(120, height);
  $('canvasDimensions').textContent = `${canvas.width} × ${canvas.height}`;

  if (sourceImage && sourceImage.complete && sourceImage.naturalWidth > 0) {
    drawCover(ctx, sourceImage, canvas.width, canvas.height);
  } else {
    const bg = ctx.createLinearGradient(0, 0, canvas.width, canvas.height);
    bg.addColorStop(0, '#26445b');
    bg.addColorStop(1, '#111d2c');
    ctx.fillStyle = bg;
    ctx.fillRect(0, 0, canvas.width, canvas.height);
    ctx.fillStyle = 'rgba(255,255,255,.08)';
    ctx.beginPath();
    ctx.arc(canvas.width * .77, canvas.height * .28, canvas.height * .28, 0, Math.PI * 2);
    ctx.fill();
  }

  const lines = resolvedTemplate().split('\n');
  const panelHeight = Math.min(canvas.height * .48, Math.max(canvas.height * .34, lines.length * canvas.height * .065 + canvas.height * .08));
  const panelTop = canvas.height - panelHeight;
  const gradient = ctx.createLinearGradient(0, panelTop - canvas.height * .08, 0, canvas.height);
  gradient.addColorStop(0, 'rgba(8,15,25,0)');
  gradient.addColorStop(.17, 'rgba(8,15,25,.78)');
  gradient.addColorStop(1, 'rgba(8,15,25,.92)');
  ctx.fillStyle = gradient;
  ctx.fillRect(0, panelTop - canvas.height * .08, canvas.width, panelHeight + canvas.height * .08);
  ctx.fillStyle = '#5bcefa';
  ctx.fillRect(canvas.width * .045, panelTop + canvas.height * .055, canvas.width * .012, panelHeight * .72);

  const left = canvas.width * .085;
  const maxWidth = canvas.width * .86;
  let fontSize = Math.max(7, canvas.width * .034);
  const lineHeight = Math.max(10, fontSize * 1.42);
  const maxLines = Math.max(2, Math.floor((canvas.height - panelTop - canvas.height * .13) / lineHeight));
  const displayLines = lines.slice(0, maxLines);
  ctx.textBaseline = 'top';
  displayLines.forEach((line, index) => {
    const isTitle = index === 0;
    const size = isTitle ? fontSize * 1.12 : fontSize;
    ctx.font = `${isTitle ? '700' : '500'} ${size}px "Segoe UI", "Microsoft YaHei", sans-serif`;
    ctx.fillStyle = isTitle ? '#8bdcf8' : '#f0f5fa';
    const wrapped = wrapText(ctx, line, maxWidth);
    wrapped.forEach((part, subIndex) => {
      const y = panelTop + canvas.height * .065 + (index + subIndex) * lineHeight;
      if (y + lineHeight < canvas.height - canvas.height * .025) ctx.fillText(part, left, y);
    });
  });

  if (!sourceImage) {
    ctx.font = `600 ${Math.max(8, canvas.width * .025)}px "Segoe UI", sans-serif`;
    ctx.fillStyle = 'rgba(255,255,255,.55)';
    ctx.textAlign = 'right';
    ctx.fillText('IMAGE PREVIEW', canvas.width * .96, canvas.height * .055);
    ctx.textAlign = 'left';
  }
}

function drawSpectrumIdle() {
  const canvas = $('spectrum');
  const ctx = canvas.getContext('2d');
  const w = canvas.width;
  const h = canvas.height;
  ctx.clearRect(0, 0, w, h);
  ctx.fillStyle = '#09111b';
  ctx.fillRect(0, 0, w, h);
  ctx.strokeStyle = 'rgba(104,143,174,.18)';
  ctx.lineWidth = 1;
  for (let x = 0; x <= w; x += w / 12) {
    ctx.beginPath(); ctx.moveTo(x, 0); ctx.lineTo(x, h); ctx.stroke();
  }
  for (let y = 0; y <= h; y += h / 5) {
    ctx.beginPath(); ctx.moveTo(0, y); ctx.lineTo(w, y); ctx.stroke();
  }
  ctx.setLineDash([5, 7]);
  ctx.strokeStyle = 'rgba(91,206,250,.38)';
  ctx.beginPath(); ctx.moveTo(0, h * .58); ctx.lineTo(w, h * .58); ctx.stroke();
  ctx.setLineDash([]);
}

async function chooseImage() {
  try {
    const path = await window.go.main.App.SelectImage();
    if (!path) return;
    const dataURL = await window.go.main.App.LoadImagePreview(path);
    const img = new Image();
    img.onload = () => {
      selectedImagePath = path;
      sourceImage = img;
      $('imageName').textContent = path.split(/[\\/]/).pop();
      renderTxCanvas();
      setResult('背景图片已载入，图文模板已合成');
    };
    img.onerror = () => setResult('无法在预览区载入所选图片', true);
    img.src = dataURL;
  } catch (error) {
    setResult(`载入图片失败：${String(error)}`, true);
  }
}

function updateModeInfo() {
  const mode = modes.find((m) => m.id === $('mode').value);
  if (!mode) return;
  $('modeInfo').textContent = `${mode.width} × ${mode.height} · ${mode.ready ? '编码器已实现' : '当前仅登记模式，编码尚未实现'}`;
  $('modeStatus').textContent = mode.ready ? `${mode.name} · 可编码` : `${mode.name} · 暂不可编码`;
  $('modeStatus').style.color = mode.ready ? '#a6dff2' : '#e5bd83';
  $('encode').disabled = !mode.ready;
  $('rxDetectedMode').textContent = mode.name;
  $('rxResolution').textContent = `${mode.width} × ${mode.height}`;
  renderTxCanvas();
}

async function encodeWAV() {
  const mode = $('mode').value;
  const spec = modes.find((m) => m.id === mode);
  if (!spec || !spec.ready) {
    setResult('当前选择的 SSTV 模式尚无可用编码器', true);
    return;
  }
  if (!selectedImagePath) {
    setResult('请先选择背景图片，再编辑发送内容', true);
    return;
  }
  const button = $('encode');
  button.disabled = true;
  setResult('正在合成画面并编码 WAV…');
  try {
    renderTxCanvas();
    const dataURL = $('txCanvas').toDataURL('image/png');
    const fileName = selectedImagePath.split(/[\\/]/).pop();
    const path = await window.go.main.App.EncodeImageData(dataURL, fileName, mode, Number($('rate').value));
    setResult(`WAV 已保存：${path}`);
  } catch (error) {
    setResult(`编码失败：${String(error)}`, true);
  } finally {
    button.disabled = false;
    updateModeInfo();
  }
}

function insertToken(token) {
  const area = $('templateText');
  const start = area.selectionStart;
  const end = area.selectionEnd;
  area.setRangeText(token, start, end, 'end');
  area.focus();
  renderTxCanvas();
}

function setupSettingsDialog() {
  const dialog = $('settingsDialog');
  $('openSettings').addEventListener('click', () => dialog.showModal());
  $('closeSettings').addEventListener('click', () => dialog.close());
  dialog.addEventListener('click', (event) => {
    if (event.target === dialog) dialog.close();
  });
}

$('choose').addEventListener('click', chooseImage);
$('mode').addEventListener('change', updateModeInfo);
$('encode').addEventListener('click', encodeWAV);
$('templateText').addEventListener('input', renderTxCanvas);
fields.forEach((id) => $(id).addEventListener('input', renderTxCanvas));
$('resetTemplate').addEventListener('click', () => {
  $('templateText').value = defaultTemplate;
  renderTxCanvas();
});
document.querySelectorAll('[data-token]').forEach((button) => {
  button.addEventListener('click', () => insertToken(button.dataset.token));
});

window.addEventListener('DOMContentLoaded', async () => {
  drawSpectrumIdle();
  renderTxCanvas();
  setupSettingsDialog();
  try {
    modes = await window.go.main.App.GetModes();
    $('mode').innerHTML = modes.map((m) => `<option value="${m.id}">${m.name} (${m.id})${m.ready ? '' : ' · 未实现'}</option>`).join('');
    const firstReady = modes.find((m) => m.ready);
    if (firstReady) $('mode').value = firstReady.id;
    updateModeInfo();
  } catch (error) {
    $('modeInfo').textContent = `读取模式列表失败：${String(error)}`;
    setResult('读取 SSTV 模式列表失败', true);
  }
});
