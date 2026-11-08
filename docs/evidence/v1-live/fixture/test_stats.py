import unittest
from stats import average

class AverageTests(unittest.TestCase):
    def test_empty(self):
        self.assertEqual(average([]), 0)
    def test_values(self):
        self.assertEqual(average([2, 4]), 3)

if __name__ == "__main__":
    unittest.main()
