import sys
from pathlib import Path

# Make the project root importable so `from perforce_exporter import ...`
# works when pytest is run from any directory.
sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
