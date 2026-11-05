// The tab's icon carries a dot when something waits: red for yours to
// send, teal for theirs to get (artboard H1). Drawn over the 32px icon on
// a canvas; the data: URL it makes is allowed by the page's CSP.
const colors = { send: '#FF0000', get: '#007480' };
const original = new Map<HTMLLinkElement, string>();
let asked = 0; // the latest call wins over an image still loading

const icons = () => [...document.querySelectorAll<HTMLLinkElement>('link[rel="icon"]')];

export function setFavicon(dot: 'send' | 'get' | null): void {
  const call = ++asked;
  for (const l of icons()) if (!original.has(l)) original.set(l, l.href);
  if (!dot) {
    for (const [l, href] of original) l.href = href;
    return;
  }
  const base = icons().find((l) => l.sizes.contains('32x32'));
  if (!base) return;
  const img = new Image();
  img.onload = () => {
    if (call !== asked) return;
    const c = document.createElement('canvas');
    c.width = c.height = 32;
    const g = c.getContext('2d');
    if (!g) return;
    g.drawImage(img, 0, 0, 32, 32);
    g.fillStyle = colors[dot];
    g.beginPath();
    g.arc(26, 6, 6, 0, 2 * Math.PI);
    g.fill();
    const url = c.toDataURL('image/png');
    for (const l of icons()) l.href = url;
  };
  img.src = original.get(base)!;
}
