"""Hello World 演示模块"""


def hello() -> str:
    """返回基本的 Hello World 消息"""
    return "Hello, World!"


def greet(name: str) -> str:
    """返回带名字的问候消息

    Args:
        name: 要问候的人名

    Returns:
        个性化问候消息
    """
    return f"Hello, {name}!"


def greet_with_time(name: str, time_of_day: str) -> str:
    """返回带时间的问候消息

    Args:
        name: 要问候的人名
        time_of_day: 时间段 (morning/afternoon/evening)

    Returns:
        带时间的问候消息
    """
    return f"Good {time_of_day}, {name}!"


if __name__ == "__main__":
    print(hello())
    print(greet("Claude"))
    print(greet_with_time("World", "morning"))
