import sys
import os.path as osp
import json
import logging
from typing import Optional
import traceback
from flask import Flask, send_from_directory


BASE_DIR = osp.dirname(osp.abspath(__file__))
PACKAGE_STORE_DIR = osp.join(BASE_DIR, "packages")
VERSION_CONFIG_PATH = osp.join(PACKAGE_STORE_DIR, "version.json")


def config_logger(name, filename: Optional[str]):
    root = logging.getLogger(name)
    root.setLevel(logging.INFO)
    if filename is None:
        handler = logging.StreamHandler(sys.stdout)
    else:
        handler = logging.FileHandler(filename)
    handler.setLevel(logging.INFO)
    formatter = logging.Formatter(
        "[%(asctime)s] [%(filename)s:%(lineno)s] [%(levelname)s] %(message)s"
    )
    handler.setFormatter(formatter)
    root.handlers = [handler]
    return root


app = Flask(__name__)
logger = config_logger("main", osp.join(BASE_DIR, "log.txt"))


@app.route("/latest_version", methods=["GET"])
def latest_version():
    try:
        with open(VERSION_CONFIG_PATH, "r", encoding="utf-8") as f:
            version_data = json.load(f)
        versions = version_data["versions"]
        versions = list(
            map(lambda v: tuple(map(int, v["version"].split("."))), versions)
        )
        versions = sorted(versions)
        return ".".join(map(lambda i: str(i), versions[-1])), 200
    except:
        logger.error(traceback.format_exc())
        return "Internal server error", 500


@app.route("/download/<version>", methods=["GET"])
def download(version):
    try:
        with open(VERSION_CONFIG_PATH, "r", encoding="utf-8") as f:
            version_data = json.load(f)
        array = version_data["versions"]
        for obj in array:
            if obj["version"] == version:
                return send_from_directory(
                    directory=PACKAGE_STORE_DIR, path=obj["path"], as_attachment=True
                )
    except:
        logger.error(traceback.format_exc())
        return "Internal server error", 500


if __name__ == "__main__":
    app.run(debug=True, host="0.0.0.0")
