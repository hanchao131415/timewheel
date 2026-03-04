"""Hello World 模块的单元测试"""

import pytest
from hello import hello, greet, greet_with_time


class TestHello:
    """测试基本 hello 函数"""

    def test_hello_returns_string(self):
        """测试返回类型是字符串"""
        result = hello()
        assert isinstance(result, str)

    def test_hello_returns_correct_message(self):
        """测试返回正确的消息"""
        assert hello() == "Hello, World!"


class TestGreet:
    """测试 greet 函数"""

    def test_greet_with_name(self):
        """测试带名字的问候"""
        assert greet("Alice") == "Hello, Alice!"

    def test_greet_with_another_name(self):
        """测试另一个名字"""
        assert greet("Bob") == "Hello, Bob!"

    def test_greet_with_empty_name(self):
        """测试空名字"""
        assert greet("") == "Hello, !"

    def test_greet_with_special_chars(self):
        """测试特殊字符名字"""
        assert greet("User123") == "Hello, User123!"


class TestGreetWithTime:
    """测试 greet_with_time 函数"""

    def test_morning_greeting(self):
        """测试早上问候"""
        assert greet_with_time("Alice", "morning") == "Good morning, Alice!"

    def test_afternoon_greeting(self):
        """测试下午问候"""
        assert greet_with_time("Bob", "afternoon") == "Good afternoon, Bob!"

    def test_evening_greeting(self):
        """测试晚上问候"""
        assert greet_with_time("Charlie", "evening") == "Good evening, Charlie!"


@pytest.mark.parametrize("name,expected", [
    ("Alice", "Hello, Alice!"),
    ("Bob", "Hello, Bob!"),
    ("", "Hello, !"),
    ("123", "Hello, 123!"),
])
def test_greet_parametrized(name, expected):
    """参数化测试 greet 函数"""
    assert greet(name) == expected
