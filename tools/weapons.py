"""Zarba Nuqtasi — weapon models (original designs) built in Blender, exported as one .glb.

usage: python3 weapons.py out.glb [preview.png]

Every gun is modelled in the game's gun space (three.js): x right, y up, the muzzle towards -z, the grip near +z.
Blender is Z-up, so a game point (X, Y, Z) is placed at Blender (X, -Z, Y); the glTF export (+Y up) turns it back.
Material names match the game's gun materials (metal, metal2, poly, wood, green, silver, steel, glass, lens, tan,
tape, bottle, red, yellow, screen, fire) so the game can swap in its own materials and weapon finishes.
"""
import bpy, bmesh, math, os, sys
from mathutils import Vector, Matrix

OUT = sys.argv[1] if len(sys.argv) > 1 else 'weapons.glb'
PREVIEW = sys.argv[2] if len(sys.argv) > 2 else ''
bpy.ops.wm.read_factory_settings(use_empty=True)
scene = bpy.context.scene

def srgb(h):
    c = [((h >> 16) & 255) / 255, ((h >> 8) & 255) / 255, (h & 255) / 255]
    return tuple((x / 12.92 if x <= .04045 else ((x + .055) / 1.055) ** 2.4) for x in c) + (1.0,)
MATS = {}
def mat(name, color, rough=.6, metal=0.):
    if name in MATS: return MATS[name]
    m = bpy.data.materials.new(name); m.use_nodes = True; b = m.node_tree.nodes['Principled BSDF']
    b.inputs['Base Color'].default_value = srgb(color); b.inputs['Roughness'].default_value = rough; b.inputs['Metallic'].default_value = metal
    MATS[name] = m; return m
COL = dict(metal=(0x2a2c2f, .38, .72), metal2=(0x3d4044, .46, .55), poly=(0x1f2022, .62, .05), wood=(0x86502a, .48, 0), silver=(0xb2b6bc, .24, .9),
           green=(0x4a5538, .7, 0), steel=(0xd3d7dc, .18, .95), glass=(0x0d1520, .04, .3), lens=(0xff2a1a, .3, 0), tan=(0x8f8566, .92, 0))
def M(n): c = COL[n]; return mat(n, c[0], c[1], c[2])

G = lambda x, y, z: Vector((x, -z, y))          # game space → Blender
PARTS = []                                        # objects of the gun being built

def finish(ob, m, bevel=0., seg=2, smooth=True):
    ob.data.materials.clear(); ob.data.materials.append(M(m))
    if bevel:
        b = ob.modifiers.new('bev', 'BEVEL'); b.width = bevel; b.segments = seg; b.limit_method = 'ANGLE'; b.angle_limit = math.radians(40); b.harden_normals = True
    dg = bpy.context.evaluated_depsgraph_get(); me = bpy.data.meshes.new_from_object(ob.evaluated_get(dg)); ob.modifiers.clear(); old = ob.data; ob.data = me; bpy.data.meshes.remove(old)
    for p in ob.data.polygons: p.use_smooth = smooth
    PARTS.append(ob); return ob

def mesh_ob(name, verts, faces):
    me = bpy.data.meshes.new(name); me.from_pydata([tuple(v) for v in verts], [], faces); me.update()
    ob = bpy.data.objects.new(name, me); scene.collection.objects.link(ob); return ob

def box(m, c, s, bevel=.003, rot=(0, 0, 0)):
    """c centre and s size in game space; rot in game axes (x, y, z) radians"""
    bpy.ops.mesh.primitive_cube_add(size=1); ob = bpy.context.active_object
    ob.scale = (s[0], s[2], s[1]); ob.rotation_euler = (rot[0], -rot[2], rot[1]); ob.location = G(*c)
    bpy.ops.object.transform_apply(location=True, rotation=True, scale=True)
    return finish(ob, m, bevel, smooth=False)

def cyl(m, c, r1, r2, length, seg=20, axis='z', bevel=.0015, rot=None):
    """a cylinder along the game z axis (default), x or y"""
    bpy.ops.mesh.primitive_cone_add(vertices=seg, radius1=r1, radius2=r2, depth=length); ob = bpy.context.active_object
    # Blender cone is along Blender Z = game y. Turn it to the requested game axis.
    if axis == 'z': ob.rotation_euler = (math.pi / 2, 0, 0)       # Blender Z → -Y (game +z toward the shooter: r1 at the muzzle side)
    elif axis == 'x': ob.rotation_euler = (0, math.pi / 2, 0)
    if rot: ob.rotation_euler = rot
    ob.location = G(*c); bpy.ops.object.transform_apply(location=True, rotation=True, scale=True)
    return finish(ob, m, bevel, smooth=True)

def sph(m, c, r, sc=(1, 1, 1)):
    bpy.ops.mesh.primitive_uv_sphere_add(segments=16, ring_count=10, radius=r); ob = bpy.context.active_object
    ob.scale = (sc[0], sc[2], sc[1]); ob.location = G(*c); bpy.ops.object.transform_apply(location=True, rotation=True, scale=True)
    return finish(ob, m, 0, smooth=True)

def prof(m, pts, thick, at=(0, 0, 0), bevel=.003, x=0):
    """side profile in game (u = +z towards the shooter, v = +y up), extruded across x, like the game's prof()"""
    n = len(pts); verts = []
    for sx in (-thick / 2, thick / 2):
        for u, v in pts: verts.append(G(at[0] + sx + x, at[1] + v, at[2] + u))
    faces = [list(range(n)), list(range(2 * n - 1, n - 1, -1))] + [[i, (i + 1) % n, n + (i + 1) % n, n + i] for i in range(n)]
    ob = mesh_ob('prof', verts, faces)
    bm = bmesh.new(); bm.from_mesh(ob.data); bmesh.ops.recalc_face_normals(bm, faces=bm.faces); bm.to_mesh(ob.data); bm.free()
    return finish(ob, m, bevel, smooth=False)

def ring(m, c, R, r, arc=math.pi * 2, axis='x', seg=24):
    """torus (trigger guards, sling loops)"""
    bpy.ops.mesh.primitive_torus_add(major_radius=R, minor_radius=r, major_segments=seg, minor_segments=8); ob = bpy.context.active_object
    if axis == 'x': ob.rotation_euler = (0, math.pi / 2, 0)
    ob.location = G(*c); bpy.ops.object.transform_apply(location=True, rotation=True, scale=True)
    return finish(ob, m, 0)

def rail(c, length, w=.022):
    box('metal', (c[0], c[1], c[2]), (w, .008, length), .0015)
    n = int(length / .01)
    for i in range(n): box('metal', (c[0], c[1] + .0055, c[2] - length / 2 + .005 + i * .01), (w + .004, .004, .005), .0008)

def trigger(z, y, guard=.022):
    prof('metal', [(0, 0), (.004, -.006), (.002, -.018), (-.004, -.022), (-.002, -.012), (-.004, 0)], .005, (0, y, z), .0008)
    prof('metal', [(-.028, 0), (-.03, -.03), (.0, -.034), (.026, -.03), (.03, 0), (.024, 0), (.02, -.025), (0, -.028), (-.022, -.025), (-.022, 0)], .006, (0, y + .004, z + .005), .0008)

GRIP = [(0, 0), (.04, 0), (.068, -.1), (.034, -.106), (.006, -.03)]

def grip(m, y, z, scale=1.0, thick=.032):
    pts = [(u * scale, v * scale) for u, v in GRIP]
    prof(m, pts, thick, (0, y, z), .006)
    for k in range(4): box(m, (0, y - .025 * scale - k * .018 * scale, z + .01 + k * .007), (thick + .002, .004, .03), .001, (0, 0, 0))

# ------------------------------------------------------------------ the guns
def rifle():
    box('metal', (0, 0, 0), (.048, .07, .34), .004); box('metal2', (0, .044, .01), (.044, .022, .29), .006)
    box('metal2', (.0245, .012, -.02), (.002, .025, .07), .0006)                                   # ejection port
    box('metal', (.03, .02, -.05), (.012, .012, .03), .002); cyl('metal', (.04, .02, -.05), .006, .006, .012, 10, 'x')  # charging handle
    box('metal', (.026, -.008, .06), (.004, .012, .05), .001)                                       # selector
    box('wood', (0, -.012, -.27), (.056, .05, .2), .012); box('wood', (0, .034, -.26), (.046, .03, .15), .01)
    for k in range(4): box('wood', (0, -.038, -.21 - k * .035), (.058, .004, .012), .001)
    cyl('metal', (0, .038, -.4), .011, .011, .14); cyl('metal', (0, .008, -.49), .0105, .0105, .32)
    box('metal', (0, .036, -.6), (.012, .05, .018), .002); cyl('metal', (0, .008, -.6), .016, .016, .03)
    cyl('metal2', (0, .008, -.67), .016, .016, .05);
    for k in range(3): box('poly', (0, .022, -.66 + k * .012), (.034, .006, .005), .0005)           # muzzle brake slots
    prof('metal', [(.035, 0), (-.035, 0), (-.075, -.09), (-.115, -.18), (-.055, -.205), (-.012, -.11)], .03, (0, -.03, -.06), .004)
    for k in range(5): box('metal2', (0, -.06 - k * .028, -.08 - k * .014), (.032, .003, .03), .0006, (-.35, 0, 0))
    grip('poly', -.03, .07); trigger(.025, -.035)
    prof('wood', [(0, .02), (.3, -.015), (.3, -.105), (.25, -.112), (0, -.04)], .04, (0, 0, .16), .008)
    box('poly', (0, -.06, .455), (.042, .1, .012), .003)
    box('metal', (0, .05, -.05), (.03, .012, .04), .003)                                            # rear sight block
    ring('metal', (0, -.028, .3), .012, .0025)
    return dict(muzzle=(0, .008, -.7), grip=(0, -.08, .1), support=(0, -.035, -.27), supportTP=(0, -.03, -.06))

def carbine():
    box('poly', (0, .01, 0), (.05, .058, .3), .005); rail((0, .046, -.07), .44, .026)
    box('poly', (0, -.038, .02), (.046, .05, .17), .007); box('metal2', (0, .004, -.28), (.06, .06, .26), .012)
    for k in range(6): box('poly', (.031, .004, -.2 - k * .03), (.002, .02, .012), .0005); box('poly', (-.031, .004, -.2 - k * .03), (.002, .02, .012), .0005)
    cyl('metal', (0, .004, -.5), .0095, .0095, .2); cyl('metal', (0, .004, -.62), .014, .014, .05)
    for k in range(3): box('metal2', (0, .004, -.6 - k * .012), (.03, .004, .004), .0005)
    box('metal', (0, .046, -.42), (.012, .05, .012), .002)
    box('poly', (0, .078, -.03), (.034, .036, .065), .008); cyl('glass', (0, .08, -.064), .013, .013, .004); sph('lens', (0, .08, -.058), .0025)
    box('metal2', (.026, .02, -.01), (.004, .02, .06), .0008)
    prof('poly', [(.03, 0), (-.03, 0), (-.046, -.15), (.012, -.155)], .028, (0, -.058, -.035), .004)
    for k in range(4): box('metal2', (0, -.08 - k * .03, -.02 - k * .005), (.03, .003, .045), .0006, (-.12, 0, 0))
    grip('poly', -.06, .07); trigger(.025, -.06)
    cyl('metal', (0, .006, .23), .016, .016, .18); box('poly', (0, -.018, .3), (.042, .085, .13), .012)
    box('poly', (0, -.07, .31), (.03, .02, .08), .004)
    return dict(muzzle=(0, .004, -.65), grip=(0, -.11, .1), support=(0, -.03, -.29), supportTP=(0, -.05, -.03))

def smg():
    box('metal', (0, 0, 0), (.05, .075, .28), .008); rail((0, .044, -.02), .2, .026)
    cyl('poly', (0, .005, -.19), .02, .02, .11); cyl('metal', (0, .005, -.27), .01, .01, .07)
    for k in range(6): box('metal2', (0, .005, -.15 - k * .015), (.044, .044, .004), .0008)
    box('metal2', (0, -.11, -.05), (.03, .17, .038), .005, (.08, 0, 0))
    for k in range(4): box('metal', (0, -.06 - k * .035, -.05 + k * .003), (.032, .004, .04), .0006, (.08, 0, 0))
    grip('poly', -.035, .07); trigger(.02, -.04)
    box('poly', (0, -.07, -.15), (.03, .08, .03), .008)
    box('metal', (.018, -.005, .23), (.01, .01, .2), .002); box('metal', (-.018, -.005, .23), (.01, .01, .2), .002); box('poly', (0, -.03, .33), (.05, .08, .014), .004)
    box('metal2', (.026, .01, -.03), (.003, .02, .05), .0006)
    return dict(muzzle=(0, .005, -.31), grip=(0, -.085, .1), support=(0, -.1, -.15), supportTP=(0, -.1, -.15))

def pistol():
    box('metal', (0, .02, 0), (.031, .036, .19), .005)
    for i in range(6): box('metal2', (0, .022, .06 + i * .006), (.033, .026, .003), .0006)
    box('metal2', (.016, .028, -.02), (.002, .014, .045), .0005)
    box('poly', (0, -.006, .006), (.029, .024, .17), .005); prof('poly', [(-.005, 0), (.045, 0), (.07, -.105), (.026, -.112)], .03, (0, -.014, .035), .006)
    for k in range(4): box('poly', (0, -.04 - k * .016, .05 + k * .005), (.032, .003, .03), .0006, (-.3, 0, 0))
    trigger(.0, -.018)
    box('metal', (0, .042, -.085), (.006, .008, .008), .001); box('metal', (0, .042, .085), (.016, .008, .008), .001)
    return dict(muzzle=(0, .02, -.1), grip=(0, -.06, .07), support=(-.03, -.066, .065), supportTP=(-.03, -.066, .065))

def deagle():
    box('silver', (0, .024, 0), (.036, .042, .25), .005); box('silver', (0, .049, -.02), (.013, .012, .2), .003)
    for i in range(7): box('metal2', (0, .026, .07 + i * .007), (.038, .03, .003), .0006)
    box('metal2', (.018, .03, -.03), (.002, .018, .06), .0005)
    box('metal2', (0, -.008, .01), (.034, .03, .21), .005); prof('poly', [(-.005, 0), (.05, 0), (.08, -.12), (.03, -.128)], .034, (0, -.018, .045), .006)
    for k in range(5): box('poly', (0, -.045 - k * .016, .06 + k * .006), (.036, .003, .032), .0006, (-.3, 0, 0))
    trigger(.0, -.026)
    cyl('silver', (0, .024, -.128), .009, .009, .01, 12)
    return dict(muzzle=(0, .024, -.13), grip=(0, -.07, .085), support=(-.034, -.076, .08), supportTP=(-.034, -.076, .08))

def sniper():
    cyl('metal', (0, .02, -.02), .022, .022, .22); box('green', (0, -.02, 0), (.055, .07, .42), .012)
    for k in range(8): box('metal2', (0, -.02, -.15 - k * .018), (.057, .05, .005), .001)
    cyl('metal', (0, .02, -.5), .016, .012, .6); box('metal2', (0, .02, -.83), (.036, .03, .06), .006)
    for k in range(3): box('poly', (0, .036, -.82 + k * .015), (.04, .006, .006), .0008)
    cyl('poly', (0, .1, -.05), .016, .016, .3); cyl('poly', (0, .1, -.24), .016, .026, .09); cyl('poly', (0, .1, .13), .022, .016, .06); cyl('glass', (0, .1, -.286), .022, .022, .004)
    cyl('poly', (0, .127, -.06), .012, .012, .03, 14, 'y'); cyl('poly', (.025, .1, -.06), .012, .012, .03, 14, 'x')
    box('metal', (0, .07, -.12), (.03, .035, .02), .004); box('metal', (0, .07, .03), (.03, .035, .02), .004)
    cyl('metal', (.035, .03, .08), .005, .005, .06, 10, 'x'); sph('metal', (.065, .03, .08), .011)
    box('metal2', (0, -.08, -.04), (.04, .07, .08), .008)
    prof('green', [(0, .03), (.32, .02), (.34, -.12), (.28, -.13), (.18, -.07), (.1, -.07), (.06, -.1), (0, -.06)], .045, (0, 0, .2), .008)
    box('poly', (0, -.03, .535), (.044, .14, .02), .004)
    grip('poly', -.05, .1); trigger(.05, -.055)
    box('metal', (.016, -.06, -.3), (.008, .008, .2), .002); box('metal', (-.016, -.06, -.3), (.008, .008, .2), .002)
    return dict(muzzle=(0, .02, -.87), grip=(0, -.1, .13), support=(0, -.055, -.22), supportTP=(0, -.055, -.18))

def scout():
    cyl('metal', (0, .02, -.02), .019, .019, .2); box('poly', (0, -.018, .02), (.046, .055, .38), .012)
    cyl('metal', (0, .02, -.45), .012, .009, .52); cyl('metal2', (0, .02, -.72), .014, .014, .04)
    for k in range(2): box('poly', (0, .033, -.715 + k * .014), (.03, .005, .005), .0006)
    cyl('metal2', (0, .085, -.04), .013, .013, .2); cyl('metal2', (0, .085, -.17), .013, .02, .06); cyl('metal2', (0, .085, .085), .019, .013, .05); cyl('glass', (0, .085, -.201), .02, .02, .004)
    cyl('metal2', (0, .105, -.04), .007, .007, .02, 12, 'y'); cyl('metal2', (.02, .085, -.04), .007, .007, .02, 12, 'x')
    box('metal', (0, .058, -.1), (.024, .03, .018), .004); box('metal', (0, .058, .03), (.024, .03, .018), .004)
    cyl('metal', (.032, .028, .07), .005, .005, .055, 10, 'x'); sph('metal', (.06, .028, .07), .01)
    box('metal2', (0, -.065, -.04), (.034, .05, .06), .008)
    prof('poly', [(0, .02), (.3, .012), (.31, -.11), (.26, -.118), (.16, -.06), (.08, -.06), (.05, -.09), (0, -.05)], .04, (0, -.004, .2), .008)
    grip('poly', -.045, .1, 1, .03); trigger(.05, -.05)
    return dict(muzzle=(0, .02, -.75), grip=(0, -.09, .13), support=(0, -.05, -.2), supportTP=(0, -.05, -.14))

def shotgun():
    box('metal', (0, .004, 0), (.05, .068, .27), .006); box('metal2', (0, .043, .005), (.044, .012, .24), .004)
    box('metal2', (.026, .012, -.03), (.002, .028, .07), .0006)
    cyl('metal', (0, .024, -.38), .0125, .0125, .5); cyl('metal2', (0, -.006, -.33), .011, .011, .4)
    cyl('metal', (0, .024, -.63), .015, .015, .012); sph('silver', (0, .041, -.6), .0045)
    box('wood', (0, -.008, -.31), (.052, .046, .16), .012)
    for i in range(6): box('wood', (0, -.008, -.25 - i * .022), (.055, .042, .006), .002)
    box('metal', (0, -.006, -.47), (.03, .03, .012), .003)                                          # barrel clamp
    grip('wood', -.03, .07); trigger(.03, -.034)
    prof('wood', [(0, .025), (.3, -.01), (.3, -.11), (.24, -.118), (0, -.045)], .042, (0, .005, .14), .008); box('poly', (0, -.042, .445), (.046, .11, .018), .006)
    ring('metal', (0, -.02, .32), .012, .0025)
    return dict(muzzle=(0, .024, -.64), grip=(0, -.08, .1), support=(0, -.03, -.31), supportTP=(0, -.03, -.1))

def lmg():
    box('metal2', (0, 0, 0), (.066, .085, .36), .008); box('metal', (0, .053, .02), (.062, .022, .25), .006)
    for k in range(5): box('metal2', (0, .065, -.06 + k * .04), (.064, .003, .006), .0008)
    box('poly', (0, .09, -.03), (.018, .04, .1), .006); box('poly', (0, .11, -.03), (.05, .012, .12), .006)
    box('poly', (0, .005, -.27), (.07, .07, .2), .014)
    for i in range(6): cyl('metal', (0, .02, -.2 - i * .03), .009, .009, .074, 10, 'x')
    cyl('metal', (0, .006, -.5), .014, .014, .34); cyl('metal', (0, .006, -.69), .019, .016, .06)
    for k in range(4): box('poly', (0, .022, -.67 - k * .01), (.04, .006, .004), .0006)
    box('metal', (0, .045, -.6), (.012, .045, .014), .002)
    box('metal', (.016, -.03, -.5), (.008, .008, .24), .002, (-.08, 0, 0)); box('metal', (-.016, -.03, -.5), (.008, .008, .24), .002, (-.08, 0, 0))
    box('green', (-.004, -.095, -.02), (.06, .11, .12), .01); box('poly', (-.004, -.05, -.02), (.064, .02, .124), .003)
    box('metal', (.034, -.02, -.02), (.008, .03, .06), .002)                                       # belt feed
    grip('poly', -.04, .09, 1, .034); trigger(.045, -.045)
    box('poly', (0, -.01, .3), (.05, .075, .24), .016); box('poly', (0, -.025, .425), (.054, .11, .02), .006)
    return dict(muzzle=(0, .006, -.73), grip=(0, -.1, .12), support=(0, -.03, -.27), supportTP=(0, -.03, -.1))

def mpistol():
    box('metal2', (0, .02, 0), (.032, .038, .18), .005); box('metal', (0, .02, -.105), (.03, .026, .04), .004)
    for i in range(3): box('metal', (0, .03, -.095 - i * .01), (.034, .006, .004), .0006)
    for i in range(5): box('metal', (0, .022, .055 + i * .006), (.034, .028, .003), .0006)
    box('poly', (0, -.006, .006), (.03, .024, .16), .005); prof('poly', [(-.005, 0), (.045, 0), (.07, -.105), (.026, -.112)], .03, (0, -.014, .035), .006)
    box('metal', (0, -.15, .07), (.024, .1, .03), .004, (-.2, 0, 0)); box('metal2', (0, -.2, .08), (.026, .012, .032), .003, (-.2, 0, 0))
    trigger(.0, -.018)
    box('metal', (0, .042, -.08), (.006, .008, .008), .001); box('metal', (0, .042, .08), (.016, .008, .008), .001)
    return dict(muzzle=(0, .02, -.13), grip=(0, -.06, .07), support=(-.03, -.066, .065), supportTP=(-.03, -.066, .065))

def knife():
    prof('steel', [(0, .016), (-.12, .016), (-.185, 0), (-.16, -.01), (0, -.016)], .005, (0, 0, -.01), .0015)
    prof('metal2', [(-.02, .016), (-.1, .016), (-.12, .012), (-.1, .009), (-.02, .009)], .0055, (0, 0, -.01), .0006)   # spine
    box('metal', (0, 0, 0), (.05, .01, .012), .003)
    box('poly', (0, 0, .06), (.024, .03, .11), .008)
    for i in range(4): box('metal2', (0, 0, .025 + i * .022), (.026, .032, .004), .001)
    cyl('metal', (0, 0, .118), .012, .012, .008, 12)
    return dict(muzzle=(0, 0, -.2), grip=(0, 0, .06), support=(0, 0, .06), supportTP=(0, 0, .06))

GUNS = dict(rifle=rifle, carbine=carbine, smg=smg, pistol=pistol, deagle=deagle, sniper=sniper, scout=scout, shotgun=shotgun, lmg=lmg, mpistol=mpistol, knife=knife)

def uv_box(ob, scale=12):
    me = ob.data
    if not me.uv_layers: me.uv_layers.new(name='UVMap')
    uv = me.uv_layers.active.data
    for p in me.polygons:
        n = p.normal; ax = max(range(3), key=lambda i: abs(n[i]))
        for li in p.loop_indices:
            co = me.vertices[me.loops[li].vertex_index].co
            u, w = [(co.y, co.z), (co.x, co.z), (co.x, co.y)][ax]; uv[li].uv = (u * scale, w * scale)

anchors = {}
for i, (gid, fn) in enumerate(GUNS.items()):
    PARTS.clear(); a = fn(); anchors[gid] = a
    for ob in PARTS: uv_box(ob)
    bpy.ops.object.select_all(action='DESELECT')
    for ob in PARTS: ob.select_set(True)
    bpy.context.view_layer.objects.active = PARTS[0]; bpy.ops.object.join()
    gun = bpy.context.active_object; gun.name = gid
    # merge parts that share a material is automatic (one mesh, one primitive per material)
    tris = sum(len(p.vertices) - 2 for p in gun.data.polygons)
    print(gid, 'triangles', tris)
    gun.location.x += 0   # all guns at the origin (each is a separate node)

bpy.ops.object.select_all(action='SELECT')
bpy.ops.export_scene.gltf(filepath=OUT, export_format='GLB', use_selection=True, export_yup=True, export_apply=True, export_animations=False)
print('wrote', OUT, os.path.getsize(OUT))

if PREVIEW:
    # lay the guns out on a table and render them
    from PIL import Image
    cols = 3
    for i, gid in enumerate(GUNS):
        ob = bpy.data.objects[gid]; ob.rotation_euler = (0, 0, math.pi / 2)   # muzzle to the left, seen from the side
        ob.location = ((i % cols) * 1.15, 0, -(i // cols) * .36)
    world = bpy.data.worlds.new('w'); scene.world = world; world.use_nodes = True; world.node_tree.nodes['Background'].inputs['Color'].default_value = srgb(0xd8dde2); world.node_tree.nodes['Background'].inputs['Strength'].default_value = 1.0
    bpy.ops.object.light_add(type='SUN', rotation=(math.radians(70), 0, math.radians(-20))); bpy.context.active_object.data.energy = 3.5
    cam_d = bpy.data.cameras.new('c'); cam = bpy.data.objects.new('c', cam_d); scene.collection.objects.link(cam)
    cam_d.type = 'ORTHO'; cam_d.ortho_scale = 3.5
    cam.location = (1.15, -5, -.54); cam.rotation_euler = (math.radians(86), 0, 0); scene.camera = cam
    scene.render.engine = 'CYCLES'; scene.cycles.samples = 24; scene.render.resolution_x, scene.render.resolution_y = 1500, 700
    scene.render.filepath = PREVIEW; bpy.ops.render.render(write_still=True); print('preview', PREVIEW)
