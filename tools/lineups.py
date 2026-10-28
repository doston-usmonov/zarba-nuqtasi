"""Tutun otish joylarini (lineup) o'yinning o'z granata fizikasi bilan qidiradi. Natija: lineups.json -> web/index.html dagi MAPS[*].lineups.
   Ishga tushirish: ZARBA_DEBUG=1 bilan server, keyin  ZN_URL=http://127.0.0.1:8080 python3 tools/lineups.py"""
import time, json
from playwright.sync_api import sync_playwright
import os, sys
URL = os.environ.get('ZN_URL', 'http://127.0.0.1:8080')   # ZARBA_DEBUG=1 bilan ishlayotgan mahalliy server
THREE_STUB = os.environ.get('THREE_STUB')                   # ixtiyoriy: chizmaydigan three.js (tezroq)
TARGETS = {
    'qumtepa':  [('A', "A — himoya yo'li", 14.5, 5), ('B', "B — himoya yo'li", 28.5, 5), ('M', "O'rta", 21.5, 10)],
    'bozor':    [('A', "A — himoya yo'li", 16, 9), ('B', "B — himoya yo'li", 27, 9), ('M', "O'rta", 21.5, 12)],
    'karvon':   [('A', "A — shimoliy yo'lak", 10, 11), ('B', "B — shimoliy yo'lak", 33, 11), ('M', "O'rta", 21.5, 12)],
    'registon': [('A', "A — himoya yo'li", 8, 7), ('B', "B — himoya yo'li", 35, 7), ('M', "Shimoliy peshtoq", 21.5, 12)],
    'saroy':    [('A', "A — qal'a yo'li", 15, 5), ('B', "B — qal'a yo'li", 28, 5), ('M', "Qal'a zinasi", 21.5, 11)],
    'zavod':    [('A', "A — idora yo'lagi", 9.5, 6), ('B', "B — idora yo'lagi", 33.5, 6), ('M', "Idora zinasi", 21.5, 9)],
}
JS = r"""([mid, targets]) => {
  const z = __zn; z.setMap(mid);
  const V = THREE.Vector3, cw = c => (c + .5)*2, d = new V(), out = [];
  const r2 = v => Math.round(v*100)/100;
  for (const [site, name, tx, tz] of targets){
    const T = { x:tx*2 + 1, z:tz*2 + 1 };
    let best = null;
    for (let cz=14; cz<42; cz++) for (let cx=1; cx<43; cx++){
      const t = z.cellT(cx, cz); if (!(t === 0 || t === 4 || t === 5 || t === 6)) continue;      // floor, sites, attacker spawn
      const px = cw(cx), pz = cw(cz), dist = Math.hypot(T.x - px, T.z - pz); if (dist < 12 || dist > 46) continue;
      const yaw = Math.atan2(-(T.x - px), -(T.z - pz));
      for (let pitch=-.15; pitch<1.25; pitch+=.012){
        z.viewDir(d, yaw, Math.min(1.45, pitch + .1));
        const pos = new V(px, 1.62, pz).addScaledVector(d, .25); pos.y -= .1;
        const vel = d.clone().multiplyScalar(17.5); pos.set(r2(pos.x), r2(pos.y), r2(pos.z)); vel.set(r2(vel.x), r2(vel.y), r2(vel.z));
        const pts = z.simNade('smoke', pos, vel), e = pts[pts.length - 1], err = Math.hypot(e.x - T.x, e.z - T.z);
        if (err > .9) continue;
        const score = dist - err*6 + (pitch > .35 ? 4 : 0);
        if (!best || score > best.score) best = { site, name, x:px, z:pz, yaw:+yaw.toFixed(4), pitch:+pitch.toFixed(4), land:[+e.x.toFixed(2), +e.z.toFixed(2)], err:+err.toFixed(2), dist:+dist.toFixed(1), score };
      }
    }
    if (best){ delete best.score; out.push(best); }
  }
  return out;
}"""
with sync_playwright() as p:
    b = p.chromium.launch(); ctx = b.new_context()
    if THREE_STUB: ctx.route('**/three.min.js', lambda r: r.fulfill(path=THREE_STUB, content_type='application/javascript'))
    pg = ctx.new_page(); pg.set_default_timeout(600000)
    pg.goto(URL + '/#zn-debug', wait_until='commit'); pg.wait_for_function("() => window.__zn")
    res = {}
    for mid, tg in TARGETS.items():
        t0 = time.time(); r = pg.evaluate(JS, [mid, tg]); res[mid] = r
        print(mid, round(time.time() - t0, 1), 's', [(x['site'], x['err'], x['dist'], x['pitch']) for x in r], 'missing', len(tg) - len(r))
    json.dump(res, open('lineups.json', 'w'), ensure_ascii=False)
