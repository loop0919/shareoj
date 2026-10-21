#!/usr/bin/env python3
"""shareoj-clar-network.drawio（質問・回答機能の照会に添える構成図）の座標と線を書く作業用スクリプト。

照会の論点は、質問・回答がどこを通り、どこに保存され、誰が閲覧・回答できるかをシステムが決めているかにある。
そこに関わる設備だけを描き、採点系、Route 53、ACM、監視と通知は描かない。
組み立ては drawio_layout.py が行う。
"""
from pathlib import Path

from drawio_layout import (BOTTOM1, BOTTOM2, BOTTOM3, CENTER, COMPUTE, DATABASE, EAST, EXTERNAL, MAIN, NETWORK, RES, RIGHT,
                           SECURITY, SHAPE, TOP, WEST, build)

OUT = Path(__file__).with_name('shareoj-clar-network.drawio')

frames = [
    ('cloud', '1', 'AWS Cloud', 'cloud', 205, 220, 750, 1115),
    ('region', 'cloud', '東京リージョン（ap-northeast-1）', 'region', 225, 250, 710, 1065),
    ('vpc', 'region', 'VPC（10.42.0.0/16）', 'vpc', 245, 770, 510, 515),
    # AZ の境目に置く線が両側の点線と重なって見えないよう、AZ のあいだを 30 空ける。
    ('az-1a', 'vpc', 'ap-northeast-1a', 'az', 265, 845, 220, 420),
    ('az-1c', 'vpc', 'ap-northeast-1c', 'az', 515, 845, 220, 420),
    ('sub-1a', 'az-1a', 'プライベートサブネット', 'private', 275, 875, 200, 375),
    ('sub-1c', 'az-1c', 'プライベートサブネット', 'private', 525, 875, 200, 375),
]

icons = [
    ('participant', '1', '参加者（質問者）', SHAPE.format(c=EXTERNAL, s='users'), 280, 50),
    ('staff', '1', 'コンテスト作成者\nwriter・tester', SHAPE.format(c=EXTERNAL, s='users'), 500, 50),
    ('viewer', '1', '閲覧者（公開後）', SHAPE.format(c=EXTERNAL, s='users'), 720, 50),
    ('apigw-web', 'region', 'Amazon API Gateway\nwww.share-oj.net', RES.format(c=NETWORK, s='api_gateway'), 500, 330),
    ('lambda-web', 'region', 'AWS Lambda\n画面の生成（Nuxt）', RES.format(c=COMPUTE, s='lambda'), 500, 500),
    ('apigw-api', 'region', 'Amazon API Gateway\napi.share-oj.net', RES.format(c=NETWORK, s='api_gateway'), 500, 670),
    ('cognito', 'region', 'Amazon Cognito', RES.format(c=SECURITY, s='cognito'), 865, 960),
    # API の Lambda は両方のサブネットに接続し、RDS は両方のサブネットを持つサブネットグループに置く。
    # RDS は Single-AZ だが、Terraform は AZ を指定しないので、どちらかの AZ に寄せずに境目に置く。
    ('lambda-api', 'vpc', 'AWS Lambda\nAPI（Go）\n閲覧・回答できる利用者を判定', RES.format(c=COMPUTE, s='lambda'), 500, 960),
    ('rds', 'vpc', 'Amazon RDS\nPostgreSQL 17・Single-AZ\n質問・回答・担当者情報を保存', RES.format(c=DATABASE, s='rds'), 500, 1140),
    ('eigw', 'vpc', 'Egress-only\nInternet Gateway', SHAPE.format(c=NETWORK, s='internet_gateway'), 680, 770),
]

edges = [
    ('edge-participant-to-apigw-web', 'participant', 'apigw-web', '質問の送信（HTTPS）', MAIN + RIGHT, BOTTOM1, (0.25, 0, 0), [(280, 190), (485, 190)], (280, 160)),
    ('edge-staff-to-apigw-web', 'staff', 'apigw-web', '閲覧・回答・公開操作（HTTPS）', MAIN + RIGHT, BOTTOM2, TOP, [], (500, 170)),
    ('edge-viewer-to-apigw-web', 'viewer', 'apigw-web', '公開後の閲覧（HTTPS）', MAIN + RIGHT, BOTTOM1, (0.75, 0, 0), [(720, 190), (515, 190)], (720, 160)),
    ('edge-apigw-web-to-lambda-web', 'apigw-web', 'lambda-web', 'Lambda 呼び出し', MAIN + RIGHT, BOTTOM2, TOP, [], None),
    ('edge-lambda-web-to-apigw-api', 'lambda-web', 'apigw-api', 'API 呼び出し（HTTPS）', MAIN + RIGHT, BOTTOM2, TOP, [], None),
    ('edge-apigw-api-to-lambda-api', 'apigw-api', 'lambda-api', 'Lambda 呼び出し', MAIN + RIGHT, BOTTOM2, TOP, [], None),
    ('edge-lambda-api-to-rds', 'lambda-api', 'rds', '読み書き（SQL）', MAIN + RIGHT, BOTTOM3, TOP, [], None),
    ('edge-lambda-api-to-cognito', 'lambda-api', 'cognito', '利用者の認証', MAIN + CENTER, EAST, WEST, [], (640, 960)),
]


if __name__ == '__main__':
    build(frames, icons, edges, OUT)
