"""構成図の .drawio を絶対座標から組み立てる、作業用スクリプトの共通部分。

座標はすべて図全体の絶対座標で書き、入れ子の枠の中に置く要素は書き出すときに親からの相対座標へ直す。
線は parent="1" に置くので、折れ点も絶対座標のまま書く。
"""
from xml.sax.saxutils import quoteattr

GROUP = 'html=1;whiteSpace=wrap;fontSize=12;shape=mxgraph.aws4.group;verticalAlign=top;align=left;spacingLeft=30;container=1;collapsible=0;recursiveResize=0;'
FRAME = {
    'cloud': GROUP + 'grIcon=mxgraph.aws4.group_aws_cloud_alt;strokeColor=#000000;fillColor=none;fontColor=#000000',
    'region': GROUP + 'grIcon=mxgraph.aws4.group_region;strokeColor=#00A4A6;fillColor=none;fontColor=#000000;dashed=1',
    'vpc': GROUP + 'grIcon=mxgraph.aws4.group_vpc2;strokeColor=#8C4FFF;fillColor=none;fontColor=#000000',
    'az': 'fillColor=none;strokeColor=#00A4A6;dashed=1;verticalAlign=top;align=left;spacingLeft=10;fontSize=12;fontColor=#000000;whiteSpace=wrap;html=1;container=1;collapsible=0;recursiveResize=0',
    'private': GROUP + 'grIcon=mxgraph.aws4.group_security_group;grStroke=0;strokeColor=#00A4A6;fillColor=#E6F6F7;fontColor=#147EBA',
}
RES = 'fontColor=#16191F;fillColor={c};strokeColor=#ffffff;verticalLabelPosition=bottom;verticalAlign=top;align=center;html=1;fontSize=12;aspect=fixed;shape=mxgraph.aws4.resourceIcon;resIcon=mxgraph.aws4.{s}'
SHAPE = 'fontColor=#16191F;fillColor={c};strokeColor=none;verticalLabelPosition=bottom;verticalAlign=top;align=center;html=1;fontSize=12;aspect=fixed;shape=mxgraph.aws4.{s}'
MAIN = 'edgeStyle=orthogonalEdgeStyle;html=1;endArrow=block;endFill=1;strokeColor=#545B64;rounded=0;labelBackgroundColor=none;verticalAlign=bottom;'
AUX = 'edgeStyle=orthogonalEdgeStyle;html=1;endArrow=block;endFill=1;strokeColor=#E7157B;rounded=0;dashed=1;labelBackgroundColor=none;verticalAlign=bottom;'
RIGHT = 'align=left;spacingLeft=6'  # 縦の線の右に出す
CENTER = 'align=center'  # 横の線の上に出す
BOTH = 'startArrow=block;startFill=1;'

COMPUTE, DATABASE, NETWORK, INTEGRATION, STORAGE, SECURITY, EXTERNAL = (
    '#ED7100', '#C925D1', '#8C4FFF', '#E7157B', '#7AA116', '#DD344C', '#242F3E')

# 線の出口と入口は (x 比率, y 比率, y のずらし量)。ずらすのは下の辺のラベルを避けるときだけで、ラベルの行数で決まる。
TOP, BOTTOM1, BOTTOM2, BOTTOM3 = (0.5, 0, 0), (0.5, 1, 24), (0.5, 1, 38), (0.5, 1, 52)
WEST, EAST = (0, 0.5, 0), (1, 0.5, 0)


def attr(text):
    return quoteattr(text, {'\n': '&#xa;'})


def build(frames, icons, edges, out):
    """frames は (id, parent, 名前, 枠の種類, x, y, 幅, 高さ)、icons は (id, parent, ラベル, スタイル, 中心 x, 中心 y)、
    edges は (id, 始点, 終点, ラベル, スタイル, 出口, 入口, 折れ点, ラベルを置く点) の並び。アイコンは 60x60。"""
    origin = {'1': (0, 0)}
    centers = {}
    cells = []
    for cid, parent, label, kind, x, y, w, h in frames:
        px, py = origin[parent]
        origin[cid] = (x, y)
        cells.append(f'<mxCell id="{cid}" value={attr(label)} style="{FRAME[kind]}" vertex="1" parent="{parent}">'
                     f'<mxGeometry x="{x - px}" y="{y - py}" width="{w}" height="{h}" as="geometry" /></mxCell>')
    for cid, parent, label, style, cx, cy in icons:
        px, py = origin[parent]
        centers[cid] = (cx, cy)
        cells.append(f'<mxCell id="{cid}" value={attr(label)} style="{style}" vertex="1" parent="{parent}">'
                     f'<mxGeometry x="{cx - 30 - px}" y="{cy - 30 - py}" width="60" height="60" as="geometry" /></mxCell>')

    def port(cid, spec):
        fx, fy, dy = spec
        cx, cy = centers[cid]
        return cx - 30 + 60 * fx, cy - 30 + 60 * fy + dy

    def constraint(prefix, spec):
        fx, fy, dy = spec
        s = f'{prefix}X={fx};{prefix}Y={fy};'
        if dy:
            s += f'{prefix}Dy={dy};{prefix}Perimeter=0;'
        return s

    for eid, src, tgt, label, style, out_port, in_port, points, at in edges:
        path = [port(src, out_port), *points, port(tgt, in_port)]
        geometry_x = ''
        if at is not None:
            # ラベルの位置は線全体の長さに対する割合（-1 が始点、1 が終点）で指定する。
            walked = 0.0
            found = None
            for (x1, y1), (x2, y2) in zip(path, path[1:]):
                on_segment = (min(x1, x2) - 0.5 <= at[0] <= max(x1, x2) + 0.5
                              and min(y1, y2) - 0.5 <= at[1] <= max(y1, y2) + 0.5)
                if found is None and on_segment:
                    found = walked + abs(at[0] - x1) + abs(at[1] - y1)
                walked += abs(x2 - x1) + abs(y2 - y1)
            if found is None:
                raise SystemExit(f'{eid}: label point {at} is not on the path {path}')
            geometry_x = f' x="{2 * found / walked - 1:.4f}"'
        array = ''.join(f'<mxPoint x="{x}" y="{y}" />' for x, y in points)
        array = f'<Array as="points">{array}</Array>' if points else ''
        cells.append(f'<mxCell id="{eid}" value={attr(label)} style="{style};{constraint("exit", out_port)}{constraint("entry", in_port)}" '
                     f'edge="1" parent="1" source="{src}" target="{tgt}">'
                     f'<mxGeometry relative="1"{geometry_x} as="geometry">{array}</mxGeometry></mxCell>')

    body = '\n    '.join(cells)
    out.write_text('<mxGraphModel adaptiveColors="auto">\n  <root>\n    <mxCell id="0" />\n    <mxCell id="1" parent="0" />\n'
                   f'    {body}\n  </root>\n</mxGraphModel>\n', encoding='utf-8')
