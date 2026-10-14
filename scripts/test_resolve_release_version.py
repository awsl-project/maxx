import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location("resolver", Path(__file__).with_name("resolve-release-version.py"))
resolver = importlib.util.module_from_spec(spec)
spec.loader.exec_module(resolver)


class ReleaseVersionTests(unittest.TestCase):
    def test_manual(self):
        self.assertEqual(resolver.resolve_version(" v1.2.3 ", []), "v1.2.3")

    def test_invalid_manual(self):
        for value in ["1.2.3", "v1.2.3-rc1", "v01.2.3", "v1.2"]:
            with self.assertRaises(ValueError):
                resolver.resolve_version(value, [])

    def test_empty(self):
        self.assertEqual(resolver.resolve_version("", []), "v0.0.1")

    def test_unordered_and_nonstable(self):
        self.assertEqual(resolver.resolve_version("", ["junk", "v9.0.0-rc1", "v1.9.99", "v1.10.2", "v1.10.1"]), "v1.10.3")

    def test_major_and_large_patch(self):
        self.assertEqual(resolver.resolve_version("", ["v1.99.999", "v2.0.99999999999999999999"]), "v2.0.100000000000000000000")

    def test_many_tags(self):
        self.assertEqual(resolver.resolve_version("", [f"v0.0.{i}" for i in range(100000)]), "v0.0.100000")


if __name__ == "__main__":
    unittest.main()
