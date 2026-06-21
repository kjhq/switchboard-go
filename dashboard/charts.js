export function renderSparkline(canvas, data, color = '#4361ee') {
  if (!canvas || !data || data.length < 2) return;
  const ctx = canvas.getContext('2d');
  const w = canvas.width = canvas.offsetWidth * 2;
  const h = canvas.height = canvas.offsetHeight * 2;
  ctx.scale(2, 2);
  const cw = canvas.offsetWidth;
  const ch = canvas.offsetHeight;
  const max = Math.max(...data, 1);
  const step = cw / (data.length - 1);

  ctx.clearRect(0, 0, cw, ch);

  ctx.beginPath();
  ctx.strokeStyle = color;
  ctx.lineWidth = 2;
  data.forEach((v, i) => {
    const x = i * step;
    const y = ch - (v / max) * (ch - 4) - 2;
    i === 0 ? ctx.moveTo(x, y) : ctx.lineTo(x, y);
  });
  ctx.stroke();

  ctx.lineTo(cw, ch);
  ctx.lineTo(0, ch);
  ctx.closePath();
  ctx.fillStyle = color + '20';
  ctx.fill();
}
