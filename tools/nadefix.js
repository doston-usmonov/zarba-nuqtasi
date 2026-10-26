// Brauzerdagi granata fizikasidan (web/index.html) test uchun namunalar yasaydi:
//   node tools/nadefix.js > testdata/nades.json
// Server (nadesim.go) ham aynan shu natijalarni berishi kerak — TestNadeSimMatchesClient.
const fs = require('fs'), path = require('path');
const THREE = require(path.join(__dirname, '../web/vendor/three.min.js'));
const html = fs.readFileSync(path.join(__dirname, '../web/index.html'), 'utf8');
const grab = name => { const i = html.indexOf('function ' + name + '('); if (i < 0) throw new Error(name); let d = 0, j = html.indexOf('{', i);
  for (let k = j; k < html.length; k++){ if (html[k] === '{') d++; else if (html[k] === '}' && --d === 0) return html.slice(i, k + 1); } };
const consts = html.match(/const NADE_G = [^;]+;/)[0];
const mapsSrc = html.slice(html.indexOf('const MAPS = {'), html.indexOf('const MAP_CH'));
const src = `
const V3 = THREE.Vector3, CELL = 2, GW = 44, GH = 44, FLOOR=0, WALL=1, CRATE=2, STACK=3, SITEA=4, SITEB=5, TSP=6, CTSP=7, CH = [0, 7, 1.0, 2.3, 0,0,0,0];
${mapsSrc}
const MAP_CH = { '#':WALL, '.':FLOOR, 'c':CRATE, 's':STACK, 'A':SITEA, 'B':SITEB, 't':TSP, 'k':CTSP };
const grid = new Uint8Array(GW*GH);
const wc = v => Math.floor(v/CELL), camera = { position:new V3(0, -999, 0) };
function cellT(cx,cz){ return (cx<0||cz<0||cx>=GW||cz>=GH) ? WALL : grid[cz*GW+cx]; }
function cellH(cx,cz){ return CH[cellT(cx,cz)]; }
${grab('rayGrid')}
${consts}
const _nd = new V3();
function sfxBounce(){}
let fired = null;
function detonate(n){ n.done = true; fired = { t:n.t, x:n.p.x, y:n.p.y, z:n.p.z, ground:n.ground }; }
${grab('surfaceY')}
${grab('nadeStep')}
${grab('nadeMove')}
// a tiny deterministic random
let seed = 12345; const rnd = () => (seed = (seed * 1103515245 + 12345) % 2147483648) / 2147483648;
const out = [];
for (const id of Object.keys(MAPS)){
  MAPS[id].rows.forEach((row, z) => { for (let x=0;x<GW;x++) grid[z*GW+x] = MAP_CH[row[x]] ?? WALL; });
  for (let i=0;i<70;i++){
    const type = ['he','molly','smoke','flash'][i % 4];
    let x, z; do { x = 2 + rnd()*84; z = 2 + rnd()*84; } while (cellH(wc(x), wc(z)) > 1.5);
    const r2 = v => Math.round(v*100)/100;
    const p = new V3(r2(x), r2(cellH(wc(x), wc(z)) + 1.5), r2(z)), yaw = rnd()*Math.PI*2, pitch = -.3 + rnd()*1.2, sp = rnd() < .3 ? 7.5 : 17.5;
    const v = new V3(r2(-Math.sin(yaw)*Math.cos(pitch)*sp), r2(Math.sin(pitch)*sp + 1), r2(-Math.cos(yaw)*Math.cos(pitch)*sp));
    const n = { type, p:p.clone(), v:v.clone(), t:0, ground:false, rest:false, restT:0, done:false };
    fired = null;
    for (let s=0; s<960 && !n.done; s++) nadeStep(n, NADE_H);
    if (!fired) fired = { t:n.t, x:n.p.x, y:n.p.y, z:n.p.z, ground:n.ground };
    out.push({ map:id, k:type, p:[p.x, p.y, p.z], v:[v.x, v.y, v.z], res:fired });
  }
}
process.stdout.write(JSON.stringify(out));
`;
new Function('THREE', 'process', src)(THREE, process);
