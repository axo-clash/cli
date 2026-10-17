#!/usr/bin/env python3
"""Generate the publication snapshots from the CLI. --check only detects drift."""
import argparse
import pathlib
import subprocess
import sys

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--check', action='store_true')
parser.add_argument('--backend', type=pathlib.Path)
args = parser.parse_args()
cli = pathlib.Path(__file__).resolve().parents[1]
backend = args.backend or cli.parent / 'application-backend'
contract = subprocess.run(['go', 'run', '.', 'export-contract'], cwd=cli, check=True, capture_output=True).stdout
targets = [backend / 'src/main/resources/bot-contract-scenarios.json', backend / 'publication-worker/fixtures/scenarios.json']
for target in targets:
    if args.check:
        if not target.exists() or target.read_bytes() != contract:
            sys.exit(f'Contract drift: {target}. Run cli/scripts/sync-contract.py.')
    else:
        target.write_bytes(contract)
print('Publication contract snapshots match the CLI.' if args.check else 'Publication contract snapshots regenerated.')
