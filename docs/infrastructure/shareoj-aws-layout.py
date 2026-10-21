#!/usr/bin/env python3
"""shareoj-aws.drawio（システム全体の構成図）の座標と線を書く作業用スクリプト。組み立ては drawio_layout.py が行う。"""
from pathlib import Path

from drawio_layout import (AUX, BOTH, BOTTOM1, BOTTOM2, CENTER, COMPUTE, DATABASE, EAST, EXTERNAL, INTEGRATION, MAIN, NETWORK,
                           RES, RIGHT, SECURITY, SHAPE, STORAGE, TOP, WEST, build)

OUT = Path(__file__).with_name('shareoj-aws.drawio')

# (id, parent, 名前, スタイル, x, y, 幅, 高さ)。座標は絶対座標。
frames = [
    ('cloud', '1', 'AWS Cloud', 'cloud', 160, 130, 1895, 1045),
    ('region', 'cloud', '東京リージョン（ap-northeast-1）', 'region', 180, 300, 1855, 855),
    ('vpc-app', 'region', 'VPC（アプリ・10.42.0.0/16）', 'vpc', 485, 500, 570, 480),
    ('az-app-1a', 'vpc-app', 'ap-northeast-1a', 'az', 505, 570, 260, 390),
    ('az-app-1c', 'vpc-app', 'ap-northeast-1c', 'az', 775, 570, 260, 390),
    ('sub-app-1a', 'az-app-1a', 'プライベートサブネット', 'private', 520, 600, 230, 345),
    ('sub-app-1c', 'az-app-1c', 'プライベートサブネット', 'private', 790, 600, 230, 345),
    ('vpc-pool', 'region', 'VPC（採点ホスト・IPv6 のみ）', 'vpc', 1275, 600, 570, 490),
    ('az-pool-1a', 'vpc-pool', 'ap-northeast-1a', 'az', 1295, 670, 260, 400),
    ('az-pool-1d', 'vpc-pool', 'ap-northeast-1d', 'az', 1565, 670, 260, 400),
    ('sub-pool-1a', 'az-pool-1a', 'プライベートサブネット', 'private', 1310, 700, 230, 355),
    ('sub-pool-1d', 'az-pool-1d', 'プライベートサブネット', 'private', 1580, 700, 230, 355),
]

# (id, parent, ラベル, スタイル, 中心 x, 中心 y)。アイコンは 60x60。
icons = [
    ('users', '1', '利用者', SHAPE.format(c=EXTERNAL, s='users'), 450, 50),
    ('discord', '1', 'Discord', SHAPE.format(c=EXTERNAL, s='internet'), 1700, 50),
    ('route53', 'cloud', 'Amazon Route 53\nshare-oj.net', RES.format(c=NETWORK, s='route_53'), 290, 215),
    ('acm', 'region', 'AWS Certificate Manager', RES.format(c=SECURITY, s='certificate_manager_3'), 290, 390),
    ('apigw-web', 'region', 'Amazon API Gateway\nwww.share-oj.net', RES.format(c=NETWORK, s='api_gateway'), 450, 390),
    ('lambda-web', 'region', 'AWS Lambda\nNuxt SSR', RES.format(c=COMPUTE, s='lambda'), 610, 390),
    ('apigw-api', 'region', 'Amazon API Gateway\napi.share-oj.net', RES.format(c=NETWORK, s='api_gateway'), 770, 390),
    ('cloudwatch', 'region', 'Amazon CloudWatch', RES.format(c=INTEGRATION, s='cloudwatch_2'), 1300, 390),
    ('sns', 'region', 'Amazon SNS', RES.format(c=INTEGRATION, s='sns'), 1500, 390),
    ('lambda-notify', 'region', 'AWS Lambda\nDiscord 通知', RES.format(c=COMPUTE, s='lambda'), 1700, 390),
    ('cognito', 'region', 'Amazon Cognito', RES.format(c=SECURITY, s='cognito'), 290, 690),
    ('eventbridge', 'region', 'Amazon EventBridge', RES.format(c=INTEGRATION, s='eventbridge'), 290, 1030),
    ('secrets', 'region', 'AWS Secrets Manager', RES.format(c=SECURITY, s='secrets_manager'), 1165, 675),
    ('sqs', 'region', 'Amazon SQS\nrequests / results', RES.format(c=INTEGRATION, s='sqs'), 1165, 875),
    ('s3', 'region', 'Amazon S3\njobs / test-data', RES.format(c=STORAGE, s='s3'), 1955, 860),
    ('rds', 'sub-app-1a', 'Amazon RDS\nPostgreSQL 17\nSingle-AZ', RES.format(c=DATABASE, s='rds'), 635, 860),
    ('ec2-primary', 'sub-pool-1a', 'Amazon EC2\nprimary・常時稼働', RES.format(c=COMPUTE, s='ec2'), 1425, 775),
    ('ec2-burst2', 'sub-pool-1a', 'Amazon EC2\nburst-2・bridge が起動', RES.format(c=COMPUTE, s='ec2'), 1425, 975),
    ('ec2-burst1', 'sub-pool-1d', 'Amazon EC2\nburst-1・bridge が起動', RES.format(c=COMPUTE, s='ec2'), 1695, 875),
    # 2つのサブネットにまたがる Lambda と、VPC の上の辺に載せるゲートウェイは、枠より後に書いて前面に出す。
    ('lambda-api', 'vpc-app', 'AWS Lambda\nGo API', RES.format(c=COMPUTE, s='lambda'), 770, 690),
    ('lambda-bridge', 'vpc-app', 'AWS Lambda\nbridge（配信・結果反映）', RES.format(c=COMPUTE, s='lambda'), 770, 860),
    ('eigw-app', 'vpc-app', 'Egress-only\nInternet Gateway', SHAPE.format(c=NETWORK, s='internet_gateway'), 960, 500),
    ('eigw-pool', 'vpc-pool', 'Egress-only\nInternet Gateway', SHAPE.format(c=NETWORK, s='internet_gateway'), 1760, 600),
]

# (id, 始点, 終点, ラベル, スタイル, 出口, 入口, 折れ点, ラベルを置く点)
edges = [
    ('edge-users-to-apigw-web', 'users', 'apigw-web', 'HTTPS', MAIN + RIGHT, BOTTOM1, TOP, [], None),
    ('edge-users-to-route53', 'users', 'route53', '名前解決', AUX + CENTER, WEST, TOP, [(290, 50)], (355, 50)),
    ('edge-acm-to-apigw-web', 'acm', 'apigw-web', '証明書', AUX + CENTER, EAST, WEST, [], None),
    ('edge-acm-to-apigw-api', 'acm', 'apigw-api', '証明書', AUX + CENTER, TOP, TOP, [(290, 335), (770, 335)], (610, 335)),
    ('edge-apigw-web-to-lambda-web', 'apigw-web', 'lambda-web', 'Lambda 統合', MAIN + CENTER, EAST, WEST, [], None),
    ('edge-lambda-web-to-apigw-api', 'lambda-web', 'apigw-api', 'API 呼び出し', MAIN + CENTER, EAST, WEST, [], None),
    ('edge-apigw-api-to-lambda-api', 'apigw-api', 'lambda-api', 'Lambda 統合', MAIN + RIGHT, BOTTOM2, TOP, [], None),
    ('edge-apigw-api-to-cloudwatch', 'apigw-api', 'cloudwatch', 'ログ・メトリクス\nAPI Gateway / Lambda / RDS / SQS / EC2', AUX + CENTER, EAST, WEST, [], (1035, 390)),
    ('edge-cloudwatch-to-sns', 'cloudwatch', 'sns', 'アラーム', AUX + CENTER, EAST, WEST, [], None),
    ('edge-sns-to-lambda-notify', 'sns', 'lambda-notify', '通知', AUX + CENTER, EAST, WEST, [], None),
    ('edge-lambda-notify-to-discord', 'lambda-notify', 'discord', 'Webhook', AUX + RIGHT, TOP, (0.5, 1, 32), [], None),
    ('edge-lambda-api-to-cognito', 'lambda-api', 'cognito', '認証', MAIN + CENTER, WEST, EAST, [], (620, 690)),
    ('edge-lambda-api-to-secrets', 'lambda-api', 'secrets', 'DB 認証情報', MAIN + CENTER, (1, 0.25, 0), WEST, [], (990, 675)),
    ('edge-lambda-api-to-s3', 'lambda-api', 's3', 'テストデータ読み書き', MAIN + CENTER, (1, 0.75, 0), BOTTOM2, [(925, 705), (925, 1110), (1955, 1110)], (1000, 1110)),
    ('edge-lambda-api-to-rds', 'lambda-api', 'rds', 'SQL', MAIN + CENTER, (0.25, 1, 38), TOP, [(755, 790), (635, 790)], (695, 790)),
    ('edge-lambda-api-to-bridge', 'lambda-api', 'lambda-bridge', '即時配信（Invoke）', MAIN + RIGHT, (0.75, 1, 38), (0.75, 0, 0), [], None),
    ('edge-bridge-to-rds', 'lambda-bridge', 'rds', 'SQL', MAIN + CENTER, WEST, EAST, [], None),
    ('edge-bridge-to-secrets', 'lambda-bridge', 'secrets', 'DB 認証情報', MAIN + CENTER, (1, 0.25, 0), (0.5, 1, 24), [(1095, 845), (1095, 780), (1165, 780)], (1130, 780)),
    ('edge-bridge-to-sqs', 'lambda-bridge', 'sqs', '要求送信・結果取込', MAIN + BOTH + CENTER, (1, 0.75, 0), WEST, [], (990, 875)),
    ('edge-eventbridge-to-bridge', 'eventbridge', 'lambda-bridge', '毎分起動', MAIN + CENTER, EAST, (0.25, 1, 38), [(755, 1030)], (400, 1030)),
    ('edge-bridge-to-s3', 'lambda-bridge', 's3', 'ジョブ保存', MAIN + RIGHT, (0.75, 1, 38), (0.75, 1, 38), [(785, 1125), (1970, 1125)], (785, 1065)),
    ('edge-sqs-to-ec2-primary', 'sqs', 'ec2-primary', '要求・結果', MAIN + BOTH + CENTER, (1, 0.25, 0), WEST, [(1225, 860), (1225, 775)], (1355, 775)),
    ('edge-sqs-to-ec2-burst1', 'sqs', 'ec2-burst1', '要求・結果', MAIN + BOTH + CENTER, EAST, WEST, [], (1615, 875)),
    ('edge-sqs-to-ec2-burst2', 'sqs', 'ec2-burst2', '要求・結果', MAIN + BOTH + CENTER, (1, 0.75, 0), WEST, [(1245, 890), (1245, 975)], (1355, 975)),
    ('edge-ec2-primary-to-s3', 'ec2-primary', 's3', '取得', MAIN + CENTER, EAST, (0, 0.25, 0), [(1895, 775), (1895, 845)], (1600, 775)),
    ('edge-ec2-burst1-to-s3', 'ec2-burst1', 's3', '取得', MAIN + CENTER, EAST, (0, 0.75, 0), [], (1790, 875)),
    ('edge-ec2-burst2-to-s3', 'ec2-burst2', 's3', '取得', MAIN + CENTER, EAST, (0.25, 1, 38), [(1940, 975)], (1600, 975)),
]


if __name__ == '__main__':
    build(frames, icons, edges, OUT)
