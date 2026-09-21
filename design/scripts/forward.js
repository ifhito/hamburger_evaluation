// Docker の中のブラウザから、Penpot の画面が自分の URL として持つ http://localhost:9001 を開けるようにする中継(検証用)。
// Penpot の画面は、PENPOT_PUBLIC_URI と違う URL で開くと「404 This page doesn't exist」になる。
// 使い方は design/README.md の「見た目の確認」を参照。
const net = require('net');
const PORT = Number(process.env.PORT || 9001); // PENPOT_PORT と同じ値
net.createServer((c) => {
  const u = net.connect(8080, 'penpot-frontend');
  c.pipe(u);
  u.pipe(c);
  c.on('error', () => u.destroy());
  u.on('error', () => c.destroy());
}).listen(PORT, '127.0.0.1');
