import os


def get_config():
    return {
        "stripe_key": os.environ["STRIPE_LIVE_KEY"],
        "database_url": os.environ["DATABASE_URL"],
        "port": int(os.environ.get("APP_PORT", "3000")),
    }


def main():
    cfg = get_config()
    print(f"starting app on port {cfg['port']}")


if __name__ == "__main__":
    main()
