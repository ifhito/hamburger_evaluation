"""stop-sensors.py の判定の関数のテスト。`python3 -m unittest discover -s .claude/hooks` で動く。"""
import importlib.util
import unittest
from pathlib import Path

spec = importlib.util.spec_from_file_location("stop_sensors", Path(__file__).with_name("stop-sensors.py"))
sensors = importlib.util.module_from_spec(spec)
spec.loader.exec_module(sensors)


class GoRelevantTest(unittest.TestCase):
    def test_Goのコードなどが変わったときは検査する(self):
        """Goのコード・go mod・SQL・sqlc設定が変わったときは検査する"""
        for path in (
            "backend-go/internal/domain/review.go",
            "backend-go/go.mod",
            "backend-go/go.sum",
            "backend-go/db/migrations/000001_create_users.up.sql",
            "backend-go/sqlc.yaml",
        ):
            self.assertTrue(sensors.go_relevant(path), path)

    def test_雛形や文書など検査の意味がないファイルでは検査しない(self):
        """環境変数の雛形・composeの例・文書・Go以外の領域では検査しない"""
        for path in (
            "backend-go/.env.example",
            "backend-go/docker-compose.override.yml.example",
            "backend-go/README.md",
            "frontend/src/main.tsx",
            "docs/agent/backend.md",
        ):
            self.assertFalse(sensors.go_relevant(path), path)


class GoVersionTest(unittest.TestCase):
    def test_go_modのgo行から版を読む(self):
        """go modのgo行から版を読む"""
        gomod = "module github.com/example/backend-go\n\ngo 1.27\n\nrequire x v1\n"
        self.assertEqual(sensors.parse_go_version(gomod), (1, 27))

    def test_パッチ版つきでも主版と副版だけを読む(self):
        """パッチ版つきでも 主版と副版だけを読む"""
        self.assertEqual(sensors.parse_go_version("go 1.25.0\ntoolchain go1.25.4\n"), (1, 25))

    def test_go_versionの出力から版を読む(self):
        """go versionの出力から版を読む"""
        self.assertEqual(sensors.parse_go_version("go version go1.19 darwin/arm64"), (1, 19))

    def test_版が読めなければNoneを返す(self):
        """版が読めなければNoneを返す"""
        self.assertIsNone(sensors.parse_go_version("何も書かれていない"))

    def test_ホストの版が必要な版以上のときだけ満たす(self):
        """ホストの版が必要な版以上なら満たす 古い・不明なら満たさない"""
        self.assertTrue(sensors.host_go_satisfies((1, 27), (1, 27)))
        self.assertTrue(sensors.host_go_satisfies((2, 0), (1, 27)))
        self.assertFalse(sensors.host_go_satisfies((1, 19), (1, 27)))
        self.assertFalse(sensors.host_go_satisfies(None, (1, 27)))

    def test_必要な版が分からないときはホストで動かす(self):
        """必要な版が分からないときは満たすとみなして ホストで動かす"""
        self.assertTrue(sensors.host_go_satisfies(None, None))


class LimitLinesTest(unittest.TestCase):
    def test_上限以下ならそのまま出す(self):
        """上限以下ならそのまま出す"""
        self.assertEqual(sensors.limit_lines("a\nb\nc", 30), "a\nb\nc")

    def test_上限を超えたら先頭だけ残して残りの行数を示す(self):
        """上限を超えたら 先頭だけ残して 残りの行数を示す"""
        text = "\n".join(str(i) for i in range(100))
        out = sensors.limit_lines(text, 30).splitlines()
        self.assertEqual(len(out), 31)
        self.assertEqual(out[0], "0")
        self.assertEqual(out[-1], "... ほか 70 行")


if __name__ == "__main__":
    unittest.main()
