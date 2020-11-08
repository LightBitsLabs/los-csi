# Copyright (C) 2020 Intel Corporation
# SPDX-License-Identifier: Apache-2.0

import logging
from argparse import ArgumentParser
from itertools import tee
from pathlib import Path
from typing import Iterable, Optional
import xml.etree.ElementTree as ET
from pytablewriter import MarkdownTableWriter
import csv

def getlines(source : str) -> Iterable[str]:
    text : Optional[str] = None
    text = Path(source).read_text()
    return text.splitlines()

def main() -> None:
    logging.basicConfig(level=logging.DEBUG)
    parser = ArgumentParser()
    parser.add_argument('source_file', help=f"input filename")
    parser.add_argument('output_file', help=f"output filename.")
    parser.add_argument('--name', default='results', help=f"Test name")
    parser.add_argument('--info', nargs='?', action='append', help=f"Test information")
    args = parser.parse_args()
    print(args)
    analyze(args.source_file, args.output_file, args.name, args.info)

def analyze(source : str, outputfilename : str, name : str, info: str):
    DB = []
    logging.info(f"Processing {source}...")
    lines, (firstline, *rest) = tee(getlines(source))
    if firstline.startswith("<?xml"):
        tree = ET.fromstringlist(lines)
        assert tree.tag == "testsuite"
        for test in tree:

            assert test.tag == "testcase"
            testname = test.attrib["name"]
            resp = test[0].tag
            result = {"failure": "FAIL", 
                        "skipped": "SKIP", 
                        "passed": "PASS"}.get(resp, "??")
            if result == "SKIP": continue
            d = []
            d.append(testname)
            d.append(result)
            DB.append(d)

    elif source.endswith('csv'):
        reader = csv.reader(lines)
        for row in reader:
            if 'SKIP' not in row and 'testname' not in row:
                DB.append(row)
    else:
        logging.error(f"Unable to recognize : {firstline}")

    logging.info(f"Writing {outputfilename} ...")
    with open(outputfilename, "w", encoding="utf-8") as f:
        f.write(f"# {name}\n\n")
        f.write(f"")
        for line in info:
            f.write(f"{line}\n\n")
        writer = MarkdownTableWriter(
            headers=["Test", "Result"],
            value_matrix = DB
        )
        writer.stream = f
        writer.write_table()

    logging.info("Done.")
        
if __name__ == "__main__":
    main()

