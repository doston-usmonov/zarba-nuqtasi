"""Zarba Nuqtasi — soldier models built in Blender (bpy), rigged, exported as .glb.

usage: python3 soldier.py T|CT out.glb [preview.png]

Blender axes: Z up, the soldier faces -Y; glTF export turns that into +Y up, facing +Z (what the game uses).
The gun arm ("R" bones) is on +X, as in the game.
"""
import bpy, bmesh, math, random, sys, os
from mathutils import Vector, Matrix, Quaternion, Euler

SIDE = sys.argv[1] if len(sys.argv) > 1 else 'T'
OUT = sys.argv[2] if len(sys.argv) > 2 else f'soldier_{SIDE}.glb'
PREVIEW = sys.argv[3] if len(sys.argv) > 3 else ''
CT = SIDE == 'CT'
HERE = os.path.dirname(os.path.abspath(__file__))
random.seed(7 if CT else 3)

bpy.ops.wm.read_factory_settings(use_empty=True)
scene = bpy.context.scene
col = scene.collection

def link(ob):
    col.objects.link(ob); return ob

def srgb(h):
    """hex sRGB → linear RGBA"""
    c = [((h >> 16) & 255) / 255, ((h >> 8) & 255) / 255, (h & 255) / 255]
    return tuple((x / 12.92 if x <= .04045 else ((x + .055) / 1.055) ** 2.4) for x in c) + (1.0,)

# ---------------------------------------------------------------- materials
PAL = {
    'T':  dict(camo=['#7a6c4d', '#5e5339', '#8f8060', '#433b29'], shirt=0x6d6248, gear=0x3f3a2b, gear2=0x4b4534, boot=0x3a2f24, skin=0x9a7258, hair=0x1d1712, lip=0x7d4c3c, knit=0x3b3a30, glove=0x262421, metal=0x2b2d30),
    'CT': dict(camo=['#4b5868', '#3a4452', '#58667a', '#2c333c'], shirt=0x3a4452, gear=0x2b323b, gear2=0x353e49, boot=0x24211e, skin=0x9c7862, hair=0x2b2119, lip=0x8f5a4a, knit=0x343e4a, glove=0x1b1c1e, metal=0x2b2d30),
}[SIDE]

def camo_png(path, cols, seed):
    from PIL import Image, ImageDraw, ImageFilter
    W = 512; rnd = random.Random(seed)
    im = Image.new('RGB', (W, W), cols[0]); d = ImageDraw.Draw(im)
    for c in cols[1:]:
        for _ in range(40):
            cx, cy = rnd.random() * W, rnd.random() * W
            for _ in range(7):
                x, y, r = cx + (rnd.random() - .5) * 90, cy + (rnd.random() - .5) * 90, 10 + rnd.random() * 26
                for ox in (-W, 0, W):
                    for oy in (-W, 0, W):
                        d.ellipse([x + ox - r, y + oy - r, x + ox + r, y + oy + r], fill=c)
    im = im.filter(ImageFilter.GaussianBlur(1.2))
    # fabric weave and grain
    px = im.load()
    for yy in range(W):
        for xx in range(W):
            r, g, b = px[xx, yy]; n = rnd.randint(-9, 9) + (4 if (xx + yy) % 4 < 2 else -4)
            px[xx, yy] = (max(0, min(255, r + n)), max(0, min(255, g + n)), max(0, min(255, b + n)))
    im.save(path)

def noise_png(path, base, amt, seed, size=256):
    from PIL import Image
    rnd = random.Random(seed); im = Image.new('RGB', (size, size)); px = im.load()
    b = [(base >> 16) & 255, (base >> 8) & 255, base & 255]
    for y in range(size):
        for x in range(size):
            n = rnd.randint(-amt, amt)
            px[x, y] = tuple(max(0, min(255, v + n)) for v in b)
    im.save(path)

def mat(name, color, rough=.8, metal=0.0, tex=None):
    m = bpy.data.materials.new(name); m.use_nodes = True
    bsdf = m.node_tree.nodes['Principled BSDF']
    bsdf.inputs['Base Color'].default_value = srgb(color) if isinstance(color, int) else color
    bsdf.inputs['Roughness'].default_value = rough; bsdf.inputs['Metallic'].default_value = metal
    if tex:
        img = bpy.data.images.load(tex); n = m.node_tree.nodes.new('ShaderNodeTexImage'); n.image = img
        m.node_tree.links.new(n.outputs['Color'], bsdf.inputs['Base Color'])
    return m

camo = os.path.join(HERE, f'camo_{SIDE}.png'); camo_png(camo, PAL['camo'], 11 if CT else 5)
skin_tex = os.path.join(HERE, f'skin_{SIDE}.png'); noise_png(skin_tex, PAL['skin'], 6, 2)
M = {
    'cloth': mat('cloth', None, .95, tex=camo) if False else None,
}
M['cloth'] = mat('cloth', 0xffffff, .95, tex=camo)
M['skin'] = mat('skin', 0xffffff, .55, tex=skin_tex)
M['gear'] = mat('gear', PAL['gear'], .85)
M['gear2'] = mat('gear2', PAL['gear2'], .88)
M['strap'] = mat('strap', 0x1e1e1c, .8)
M['boot'] = mat('boot', PAL['boot'], .6)
M['sole'] = mat('sole', 0x141312, .9)
M['hair'] = mat('hair', PAL['hair'], 1.0)
M['lip'] = mat('lip', PAL['lip'], .5)
M['eye'] = mat('eye', 0xe6e1d7, .2)
M['iris'] = mat('iris', 0x2a1d14, .15)
M['knit'] = mat('knit', PAL['knit'], 1.0)
M['metal'] = mat('metal', PAL['metal'], .4, .6)
M['lens'] = mat('lens', 0x0e1216, .06, .5)
M['glove'] = mat('glove', PAL['glove'], .8)
M['helm'] = mat('helm', 0x343e4a, .6)

# ---------------------------------------------------------------- helpers
def apply_mods(ob):
    dg = bpy.context.evaluated_depsgraph_get()
    me = bpy.data.meshes.new_from_object(ob.evaluated_get(dg))
    ob.modifiers.clear(); old = ob.data; ob.data = me; bpy.data.meshes.remove(old)
    return ob

def shade_smooth(ob):
    for p in ob.data.polygons: p.use_smooth = True

def set_mat(ob, m):
    ob.data.materials.clear(); ob.data.materials.append(m)

def prim(kind, name, loc=(0, 0, 0), scale=(1, 1, 1), rot=(0, 0, 0), m=None, subd=0, bevel=0, smooth=True, **kw):
    if kind == 'cube': bpy.ops.mesh.primitive_cube_add(size=1)
    elif kind == 'sphere': bpy.ops.mesh.primitive_uv_sphere_add(segments=kw.get('seg', 20), ring_count=kw.get('rings', 12), radius=1)
    elif kind == 'cyl': bpy.ops.mesh.primitive_cylinder_add(vertices=kw.get('seg', 16), radius=1, depth=1)
    elif kind == 'torus': bpy.ops.mesh.primitive_torus_add(major_radius=kw.get('R', 1), minor_radius=kw.get('r', .2), major_segments=kw.get('seg', 24), minor_segments=8)
    elif kind == 'cone': bpy.ops.mesh.primitive_cone_add(vertices=kw.get('seg', 12), radius1=1, radius2=kw.get('r2', 0), depth=1)
    ob = bpy.context.active_object; ob.name = name
    ob.location = loc; ob.scale = scale; ob.rotation_euler = rot
    bpy.ops.object.transform_apply(location=False, rotation=True, scale=True)
    if bevel:
        b = ob.modifiers.new('bev', 'BEVEL'); b.width = bevel; b.segments = 2; b.limit_method = 'NONE'
    if subd:
        s = ob.modifiers.new('sub', 'SUBSURF'); s.levels = subd; s.render_levels = subd
    if bevel or subd: apply_mods(ob)
    if smooth: shade_smooth(ob)
    if m: set_mat(ob, m)
    ob['bone'] = kw.get('bone', '')
    return ob

def half_sphere(name, loc, scale, m, cut=.5, seg=24, rings=14, bone='head'):
    bm = bmesh.new(); bmesh.ops.create_uvsphere(bm, u_segments=seg, v_segments=rings, radius=1)
    bmesh.ops.delete(bm, geom=[v for v in bm.verts if v.co.z < 1 - 2 * cut - 1e-4], context='VERTS')
    me = bpy.data.meshes.new(name); bm.to_mesh(me); bm.free()
    ob = link(bpy.data.objects.new(name, me)); ob.location = loc; ob.scale = scale
    bpy.context.view_layer.objects.active = ob; ob.select_set(True)
    bpy.ops.object.select_all(action='DESELECT'); ob.select_set(True); bpy.context.view_layer.objects.active = ob
    bpy.ops.object.transform_apply(location=False, rotation=True, scale=True)
    s = ob.modifiers.new('solid', 'SOLIDIFY'); s.thickness = .006; apply_mods(ob)
    shade_smooth(ob); set_mat(ob, m); ob['bone'] = bone
    return ob

# ---------------------------------------------------------------- skeleton (one place for joint positions)
J = {
    'pelvis': (0, 0, .93), 'waist': (0, 0, 1.06), 'chest': (0, 0, 1.25), 'upchest': (0, 0, 1.40), 'neck0': (0, 0, 1.485), 'neck1': (0, 0, 1.565),
    'headtop': (0, 0, 1.80),
}
for s, sx in (('R', 1), ('L', -1)):
    J['sh' + s] = (sx * .175, .005, 1.425); J['el' + s] = (sx * .395, .02, 1.215); J['wr' + s] = (sx * .575, .0, 1.04); J['hd' + s] = (sx * .65, -.005, .965)
    J['hip' + s] = (sx * .095, 0, .89); J['kn' + s] = (sx * .1, -.01, .5); J['an' + s] = (sx * .1, .015, .1); J['toe' + s] = (sx * .1, -.15, .03)
V = lambda k: Vector(J[k])

# ---------------------------------------------------------------- body: skin modifier over a joint graph, then subdivision
def body():
    pts, rad, edges = [], [], []
    def P(co, r):
        pts.append(co); rad.append(r); return len(pts) - 1
    def E(a, b): edges.append((a, b))
    lerp = lambda a, b, t: tuple(a[i] + (b[i] - a[i]) * t for i in range(3))
    p0 = P(J['pelvis'], (.15, .115)); w = P(J['waist'], (.138, .1)); c = P(J['chest'], (.165, .112)); uc = P(J['upchest'], (.172, .105))
    n0 = P(J['neck0'], (.06, .06)); n1 = P(J['neck1'], (.056, .055))
    E(p0, w); E(w, c); E(c, uc); E(uc, n0); E(n0, n1)
    for s in 'RL':
        sh = P(J['sh' + s], (.068, .068)); E(uc, sh)
        m1 = P(lerp(J['sh' + s], J['el' + s], .45), (.058, .055)); E(sh, m1)
        el = P(J['el' + s], (.046, .046)); E(m1, el)
        m2 = P(lerp(J['el' + s], J['wr' + s], .4), (.047, .042)); E(el, m2)
        wr = P(J['wr' + s], (.034, .028)); E(m2, wr)
        hp = P(J['hip' + s], (.09, .09)); E(p0, hp)
        t1 = P(lerp(J['hip' + s], J['kn' + s], .45), (.082, .08)); E(hp, t1)
        kn = P(J['kn' + s], (.06, .062)); E(t1, kn)
        cf = P(lerp(J['kn' + s], J['an' + s], .3), (.058, .062)); E(kn, cf)
        an = P(J['an' + s], (.042, .042)); E(cf, an)
    me = bpy.data.meshes.new('body'); me.from_pydata(pts, edges, []); ob = link(bpy.data.objects.new('body', me))
    sk = ob.modifiers.new('skin', 'SKIN'); sk.use_smooth_shade = True; sk.branch_smoothing = .6
    sv = me.skin_vertices[0].data
    for i, r in enumerate(rad): sv[i].radius = r
    sv[0].use_root = True
    sub = ob.modifiers.new('sub', 'SUBSURF'); sub.levels = 2
    apply_mods(ob)
    dec = ob.modifiers.new('dec', 'DECIMATE'); dec.ratio = .6; apply_mods(ob); shade_smooth(ob)
    # materials by region: shirt above the belt, trousers below, bare skin on the neck (and rolled sleeves for attackers)
    for m in ('cloth', 'skin'): ob.data.materials.append(M[m])
    for p in ob.data.polygons:
        x, y, z = p.center
        skin = z > 1.5 and abs(x) < .09
        if not CT and abs(x) > .45 and z > .95: skin = True     # forearms below the rolled sleeves
        if abs(x) > .56 and z > .95: skin = True                 # wrists (hands are separate)
        p.material_index = 1 if skin else 0
    ob['bone'] = ''
    return ob

# ---------------------------------------------------------------- head with a face
def head():
    hc = Vector((0, 0, 1.665))
    bm = bmesh.new(); bmesh.ops.create_uvsphere(bm, u_segments=20, v_segments=13, radius=.1)
    for v in bm.verts:
        x, y, z = v.co
        x *= .8; y *= .98; z *= 1.13
        if z < 0:                                     # jaw narrows, chin comes forward
            t = -z / .113; x *= 1 - .22 * t
            if y < 0: y *= 1 + .1 * t
        if y < -.04 and z < .03 and z > -.06: y -= .006   # cheek bones
        if y < -.05 and -.005 < z < .045 and abs(x) < .05: y += .012 * (1 - abs(x) / .05)  # eye sockets
        if y > .03 and z > -.02: y += .008            # back of the skull
        v.co = Vector((x, y, z)) + hc
    me = bpy.data.meshes.new('head'); bm.to_mesh(me); bm.free()
    ob = link(bpy.data.objects.new('head', me)); s = ob.modifiers.new('sub', 'SUBSURF'); s.levels = 1; apply_mods(ob); shade_smooth(ob); set_mat(ob, M['skin']); ob['bone'] = 'head'
    parts = [ob]
    # nose: bridge and tip
    parts.append(prim('cube', 'nose', (0, -.098, 1.668), (.02, .028, .05), (math.radians(-14), 0, 0), M['skin'], subd=2, bone='head'))
    parts.append(prim('sphere', 'nosetip', (0, -.108, 1.645), (.016, .014, .013), m=M['skin'], seg=10, rings=7, bone='head'))
    for sx in (1, -1):
        parts.append(prim('sphere', 'ear', (sx * .08, .0, 1.665), (.012, .022, .03), m=M['skin'], seg=10, rings=7, bone='head'))
        parts.append(prim('sphere', 'eye', (sx * .031, -.083, 1.688), (.0125, .0125, .0125), m=M['eye'], seg=10, rings=7, bone='head'))
        parts.append(prim('sphere', 'iris', (sx * .031, -.0945, 1.688), (.0065, .004, .0065), m=M['iris'], seg=10, rings=7, bone='head'))
        parts.append(prim('cube', 'brow', (sx * .033, -.092, 1.708), (.034, .008, .007), (0, math.radians(sx * 8), math.radians(sx * -6)), M['hair'], subd=1, bone='head'))
        parts.append(prim('sphere', 'lid', (sx * .031, -.085, 1.694), (.0135, .0125, .007), m=M['skin'], seg=10, rings=7, bone='head'))
    parts.append(prim('cube', 'lips', (0, -.092, 1.612), (.04, .012, .009), m=M['lip'], subd=2, bone='head'))
    return parts

def hand(s, mtl):
    """a relaxed gloved/bare hand pointing from the wrist along the forearm"""
    wr, hd = V('wr' + s), V('hd' + s); d = (hd - wr).normalized()
    q = d.to_track_quat('Z', 'Y')   # local +Z along the hand
    parts = []
    def part(kind, loc, sc, rot=(0, 0, 0), **kw):
        ob = prim(kind, 'hand', (0, 0, 0), sc, rot, m=mtl, bone='hand' + s, **kw)
        ob.matrix_world = Matrix.Translation(wr) @ q.to_matrix().to_4x4() @ Matrix.Translation(Vector(loc))
        bpy.context.view_layer.update(); bpy.ops.object.select_all(action='DESELECT'); ob.select_set(True); bpy.context.view_layer.objects.active = ob
        bpy.ops.object.transform_apply(location=True, rotation=True, scale=True)
        parts.append(ob)
    sx = 1 if s == 'R' else -1
    part('cube', (0, 0, .042), (.056, .03, .09), bevel=.012)
    part('cube', (0, .006, .1), (.052, .026, .052), (math.radians(18), 0, 0), bevel=.01)
    part('cube', (sx * .033, -.012, .045), (.019, .019, .058), (0, math.radians(sx * -20), 0), bevel=.008)
    return parts

def boot(s):
    a = V('an' + s); sx = 1 if s == 'R' else -1
    parts = [prim('cyl', 'shaft', (a.x, a.y, a.z + .02), (.058, .058, .13), m=M['boot'], seg=14, bone='foot' + s, bevel=.012),
             prim('cube', 'foot', (a.x, a.y - .045, a.z - .052), (.1, .25, .085), m=M['boot'], bevel=.03, bone='foot' + s),
             prim('sphere', 'toe', (a.x, a.y - .15, a.z - .06), (.05, .045, .04), m=M['boot'], seg=12, rings=7, bone='foot' + s),
             prim('cube', 'sole', (a.x, a.y - .05, a.z - .095), (.108, .275, .022), m=M['sole'], bevel=.006, bone='foot' + s)]
    return parts

# ---------------------------------------------------------------- gear
def gear():
    g = []
    belt = prim('torus', 'belt', (0, 0, 1.0), (1, .78, .7), m=M['strap'], R=.146, r=.02, seg=28, bone='hips'); g.append(belt)
    g.append(prim('cube', 'buckle', (0, -.118, 1.0), (.05, .012, .04), m=M['metal'], bevel=.004, bone='hips'))
    for s in 'RL':
        sx = 1 if s == 'R' else -1
        g += hand(s, M['glove'] if CT else M['skin'])
        g += boot(s)
        # thigh cargo pocket
        k, h = V('kn' + s), V('hip' + s)
        g.append(prim('cube', 'pocket', (sx * .172, -.005, .68), (.035, .11, .13), m=M['cloth'], bevel=.012, bone='thigh' + s))
        if CT:
            g.append(prim('cube', 'kneepad', (k.x, k.y - .06, k.z + .005), (.1, .045, .11), m=M['gear'], subd=2, bone='shin' + s))
            g.append(prim('cube', 'elbowpad', (V('el' + s).x, .055, V('el' + s).z), (.06, .03, .07), m=M['gear'], subd=2, bone='forearm' + s))
    if CT:
        # plate carrier, cummerbund, pouches, radio, hydration pack
        g.append(prim('cube', 'plateF', (0, -.125, 1.29), (.34, .05, .3), m=M['gear'], bevel=.018, bone='chest'))
        g.append(prim('cube', 'plateB', (0, .118, 1.3), (.34, .05, .32), m=M['gear'], bevel=.018, bone='chest'))
        for sx in (1, -1): g.append(prim('cube', 'cumm', (sx * .168, 0, 1.2), (.045, .2, .17), m=M['gear'], bevel=.012, bone='chest'))
        for sx in (1, -1): g.append(prim('cube', 'strapS', (sx * .115, 0, 1.44), (.07, .26, .02), m=M['strap'], bevel=.005, bone='chest'))
        for x in (-.085, 0, .085):
            g.append(prim('cube', 'pouch', (x, -.172, 1.2), (.07, .045, .11), m=M['gear2'], bevel=.01, bone='chest'))
            g.append(prim('cube', 'flap', (x, -.175, 1.26), (.075, .05, .024), m=M['gear'], bevel=.006, bone='chest'))
        g.append(prim('cube', 'admin', (-.08, -.165, 1.37), (.1, .035, .07), m=M['gear2'], bevel=.008, bone='chest'))
        g.append(prim('cube', 'radio', (.13, -.16, 1.38), (.04, .04, .11), m=M['strap'], bevel=.006, bone='chest'))
        g.append(prim('cyl', 'antenna', (.14, -.15, 1.49), (.004, .004, .14), m=M['strap'], seg=6, bone='chest'))
        g.append(prim('cube', 'pack', (0, .165, 1.31), (.21, .07, .25), m=M['gear2'], bevel=.025, bone='chest'))
        g.append(prim('cube', 'holster', (-.15 * -1, -.02, .76), (.04, .07, .15), m=M['strap'], bevel=.01, bone='thighR'))
        # helmet with headset and glasses
        g.append(half_sphere('helmet', (0, .008, 1.712), (.112, .13, .118), M['helm'], cut=.5))
        g.append(prim('torus', 'rim', (0, .008, 1.712), (1, 1.16, 1), m=M['helm'], R=.11, r=.007, seg=32, bone='head'))
        g.append(prim('torus', 'collar', (0, .006, 1.505), (1, .95, 1.6), m=M['cloth'], R=.062, r=.018, seg=20, bone='neck'))
        g.append(prim('cube', 'nvg', (0, -.1, 1.81), (.045, .022, .034), m=M['metal'], bevel=.005, bone='head'))
        for sx in (1, -1):
            g.append(prim('cube', 'rail', (sx * .108, 0, 1.73), (.012, .13, .025), m=M['metal'], bevel=.003, bone='head'))
            g.append(prim('cyl', 'cup', (sx * .088, .005, 1.665), (.03, .03, .032), (0, math.pi / 2, 0), M['strap'], seg=16, bone='head', bevel=.006))
            g.append(prim('cube', 'arm', (sx * .066, -.064, 1.689), (.006, .09, .006), m=M['strap'], bone='head'))
        g.append(prim('cube', 'glasses', (0, -.113, 1.772), (.1, .018, .03), (math.radians(-35), 0, 0), M['lens'], bevel=.007, bone='head'))
    else:
        # chest rig with magazine pouches, X straps on the back, scarf, knit cap and beard
        g.append(prim('cube', 'rig', (0, -.125, 1.2), (.32, .055, .17), m=M['gear'], bevel=.015, bone='chest'))
        for x in (-.11, -.037, .037, .11):
            g.append(prim('cube', 'pouch', (x, -.163, 1.2), (.065, .04, .12), m=M['gear2'], bevel=.01, bone='chest'))
            g.append(prim('cube', 'flap', (x, -.166, 1.265), (.068, .045, .022), m=M['gear'], bevel=.005, bone='chest'))
        for sx in (1, -1):
            g.append(prim('cube', 'strapF', (sx * .1, -.11, 1.38), (.05, .02, .24), (math.radians(-8), 0, math.radians(sx * 12)), M['strap'], bevel=.004, bone='chest'))
            g.append(prim('cube', 'strapB', (0, .118, 1.27), (.045, .02, .3), (0, math.radians(sx * 32), 0), M['strap'], bevel=.004, bone='chest'))
        g.append(prim('cube', 'pack', (0, .15, 1.28), (.2, .07, .22), m=M['gear2'], bevel=.025, bone='chest'))
        g.append(prim('torus', 'scarf', (0, -.005, 1.505), (1, .92, 1.1), m=M['knit'], R=.064, r=.022, seg=20, bone='neck'))
        g.append(half_sphere('cap', (0, .004, 1.698), (.088, .104, .104), M['knit'], cut=.55))
        g.append(prim('cyl', 'capband', (0, .004, 1.71), (.09, .102, .04), m=M['knit'], seg=24, bone='head'))
        g.append(prim('sphere', 'beard', (0, -.042, 1.598), (.074, .06, .05), m=M['hair'], seg=14, rings=9, bone='head'))
        g.append(prim('cube', 'lipsT', (0, -.1, 1.617), (.036, .01, .008), m=M['lip'], subd=1, bone='head'))
        g.append(prim('cube', 'mustache', (0, -.101, 1.63), (.052, .012, .012), m=M['hair'], bevel=.004, bone='head'))
    return g

# ---------------------------------------------------------------- armature
BONES = [  # name, head, tail, parent
    ('hips', 'pelvis', 'waist', None), ('spine', 'waist', 'chest', 'hips'), ('chest', 'chest', 'neck0', 'spine'),
    ('neck', 'neck0', 'neck1', 'chest'), ('head', 'neck1', 'headtop', 'neck'),
]
for s in 'RL':
    BONES += [('upperarm' + s, 'sh' + s, 'el' + s, 'chest'), ('forearm' + s, 'el' + s, 'wr' + s, 'upperarm' + s), ('hand' + s, 'wr' + s, 'hd' + s, 'forearm' + s),
              ('thigh' + s, 'hip' + s, 'kn' + s, 'hips'), ('shin' + s, 'kn' + s, 'an' + s, 'thigh' + s), ('foot' + s, 'an' + s, 'toe' + s, 'shin' + s)]

def armature():
    arm = bpy.data.armatures.new('rig'); ob = link(bpy.data.objects.new('rig', arm))
    bpy.ops.object.select_all(action='DESELECT'); bpy.context.view_layer.objects.active = ob; ob.select_set(True)
    bpy.ops.object.mode_set(mode='EDIT')
    for name, h, t, par in BONES:
        b = arm.edit_bones.new(name); b.head = V(h); b.tail = V(t)
        if par: b.parent = arm.edit_bones[par]; b.use_connect = False
        b.roll = 0
    bpy.ops.object.mode_set(mode='OBJECT')
    return ob

def seg_dist(p, a, b):
    ab = b - a; t = max(0, min(1, (p - a).dot(ab) / ab.length_squared)); return (a + ab * t - p).length

def skin_weights(ob, rig):
    """body: smooth weights from distance to bone segments (no bleeding between the legs or arms)"""
    segs = {n: (V(h), V(t)) for n, h, t, _ in BONES}
    for n in segs: ob.vertex_groups.new(name=n)
    for v in ob.data.vertices:
        p = v.co; cands = []
        for n, (a, b) in segs.items():
            if n.endswith('R') and p.x < -.03: continue
            if n.endswith('L') and p.x > .03: continue
            if n.startswith(('thigh', 'shin', 'foot')) and p.z > 1.0: continue
            if n.startswith(('upperarm', 'forearm', 'hand')) and p.z < .9: continue
            d = seg_dist(p, a, b); cands.append((d, n))
        cands.sort(); best = cands[:3]
        ws = [(1 / (d + .012) ** 4, n) for d, n in best]; tot = sum(w for w, _ in ws)
        for w, n in ws:
            if w / tot > .02: ob.vertex_groups[n].add([v.index], w / tot, 'REPLACE')

def rigid(ob, bone):
    for n, *_ in BONES: ob.vertex_groups.new(name=n)
    ob.vertex_groups[bone].add([v.index for v in ob.data.vertices], 1.0, 'REPLACE')

def uv_box(ob, scale=6):
    """box projection: good enough for tiling camo and fabric noise"""
    me = ob.data
    if not me.uv_layers: me.uv_layers.new(name='UVMap')
    uv = me.uv_layers.active.data
    for p in me.polygons:
        n = p.normal; ax = max(range(3), key=lambda i: abs(n[i]))
        for li in p.loop_indices:
            co = me.vertices[me.loops[li].vertex_index].co
            u, w = [(co.y, co.z), (co.x, co.z), (co.x, co.y)][ax]
            uv[li].uv = (u * scale, w * scale)

# ---------------------------------------------------------------- build
rig = armature()
parts = [body()] + head() + gear()
import collections
cnt = collections.Counter()
for ob in parts: cnt[ob.name.split('.')[0]] += sum(len(p.vertices) - 2 for p in ob.data.polygons)
print(cnt.most_common(12))
for ob in parts:
    uv_box(ob, 5 if ob.name.startswith('body') else 8)
    if ob.name.startswith('body'): skin_weights(ob, rig)
    else: rigid(ob, ob['bone'] or 'chest')
# one mesh, one skin
bpy.ops.object.select_all(action='DESELECT')
for ob in parts: ob.select_set(True)
bpy.context.view_layer.objects.active = parts[0]
bpy.ops.object.join()
soldier = bpy.context.active_object; soldier.name = 'soldier_' + SIDE
mod = soldier.modifiers.new('rig', 'ARMATURE'); mod.object = rig
soldier.parent = rig
tris = sum(len(p.vertices) - 2 for p in soldier.data.polygons)
print('triangles', tris, 'materials', len(soldier.data.materials))

bpy.ops.object.select_all(action='DESELECT'); soldier.select_set(True); rig.select_set(True)
bpy.ops.export_scene.gltf(filepath=OUT, export_format='GLB', use_selection=True, export_yup=True, export_skins=True,
                          export_animations=False, export_apply=False, export_image_format='JPEG', export_jpeg_quality=85)
print('wrote', OUT, os.path.getsize(OUT))

# ---------------------------------------------------------------- preview render (posed: walking with a rifle)
if PREVIEW:
    pb = rig.pose.bones
    def rot(n, x=0, y=0, z=0):
        pb[n].rotation_mode = 'XYZ'; pb[n].rotation_euler = (math.radians(x), math.radians(y), math.radians(z))
    rot('thighR', -20); rot('shinR', 16); rot('footR', 4); rot('thighL', 16); rot('shinL', 26); rot('footL', -12)
    rot('chest', 3, 0, 0); rot('head', -2, 0, 6)
    def ik(bone, tgt, pole):
        e = link(bpy.data.objects.new('t_' + bone, None)); e.location = tgt
        p = link(bpy.data.objects.new('p_' + bone, None)); p.location = pole
        c = pb[bone].constraints.new('IK'); c.target = e; c.pole_target = p; c.chain_count = 2; c.pole_angle = math.radians(PA)
    PA = float(os.environ.get('PA', '90'))
    ik('forearmR', (.13, -.27, 1.3), (.55, .05, .75)); ik('forearmL', (-.03, -.56, 1.33), (-.5, -.2, .75))
    # a stand-in rifle (preview only)
    bpy.ops.mesh.primitive_cube_add(size=1); gun = bpy.context.active_object; gun.scale = (.05, .9, .075)
    gun.location = (.08, -.48, 1.36); gun.data.materials.append(M['metal'])
    # ground, light, camera
    bpy.ops.mesh.primitive_plane_add(size=8); gr = bpy.context.active_object; gr.data.materials.append(mat('ground', 0xc8b48a, .9))
    world = bpy.data.worlds.new('w'); scene.world = world; world.use_nodes = True; world.node_tree.nodes['Background'].inputs['Color'].default_value = srgb(0xbfd2e6); world.node_tree.nodes['Background'].inputs['Strength'].default_value = .8
    bpy.ops.object.light_add(type='SUN', rotation=(math.radians(50), math.radians(10), math.radians(-35))); bpy.context.active_object.data.energy = 3.2
    scene.render.engine = 'CYCLES'; scene.cycles.samples = 40; scene.cycles.device = 'CPU'
    scene.render.resolution_x, scene.render.resolution_y = 640, 760
    shots = []
    views = [(-20, 3.1, 1.15, 1.0), (60, 3.1, 1.15, 1.0), (-8, 1.0, 1.66, 1.62)]
    if os.environ.get('QUICK'): views = views[:2]; scene.cycles.samples = 12
    for i, (ang, dist, z, look) in enumerate(views):
        a = math.radians(ang)
        cam_data = bpy.data.cameras.new('cam'); cam = link(bpy.data.objects.new('cam', cam_data)); cam_data.lens = 50 if i < 2 else 60
        cam.location = (math.sin(a) * dist, -math.cos(a) * dist, z)
        d = Vector((0, 0, look)) - cam.location; cam.rotation_euler = d.to_track_quat('-Z', 'Y').to_euler()
        scene.camera = cam; path = PREVIEW.replace('.png', f'_{i}.png'); scene.render.filepath = path
        bpy.ops.render.render(write_still=True); shots.append(path)
    from PIL import Image
    ims = [Image.open(p) for p in shots]; W, H = ims[0].size; out = Image.new('RGB', (W * len(ims), H))
    for k, im in enumerate(ims): out.paste(im, (k * W, 0))
    out.save(PREVIEW); print('preview', PREVIEW)
