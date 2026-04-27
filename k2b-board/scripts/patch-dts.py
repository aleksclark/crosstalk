#!/usr/bin/env python3
"""Patch K2B DTS: replace spidev@1 with ili9341@1 under spi@5011000.

Uses numeric phandle references (not &pio labels) so the patched DTS
survives dtc recompilation without losing GPIO references.
"""
import sys

with open("/tmp/k2b.dts") as f:
    lines = f.readlines()

# Find the pio phandle by looking for the pinctrl@300b000 node's phandle.
# It's referenced by other gpio properties as a numeric value.
# We extract it from existing gpio references in the DTS.
pio_phandle = None
for line in lines:
    stripped = line.strip()
    # Look for cd-gpios or similar known pio reference
    if "cd-gpios" in stripped or "reset-gpios" in stripped or "gpios" in stripped:
        # Extract first hex value after <
        import re
        m = re.search(r"<(0x[0-9a-fA-F]+)\s+0x0", stripped)
        if m:
            pio_phandle = m.group(1)
            break

if not pio_phandle:
    # Fallback: scan for pinctrl@300b000 phandle
    in_pinctrl = False
    for line in lines:
        if "pinctrl@300b000" in line:
            in_pinctrl = True
        if in_pinctrl and "phandle" in line:
            import re
            m = re.search(r"<(0x[0-9a-fA-F]+)>", line)
            if m:
                pio_phandle = m.group(1)
                break

if not pio_phandle:
    pio_phandle = "0x16"
    print(f"WARNING: Could not find pio phandle, using default {pio_phandle}", file=sys.stderr)

print(f"Using pio phandle: {pio_phandle}")

out = []
i = 0
replaced = False

while i < len(lines):
    line = lines[i]
    stripped = line.strip()

    if "spidev@1" in stripped and "{" in stripped and not replaced:
        indent = line[: len(line) - len(line.lstrip())]
        depth = 1
        i += 1
        while i < len(lines) and depth > 0:
            if "{" in lines[i]:
                depth += 1
            if "}" in lines[i]:
                depth -= 1
            i += 1

        # DC = PC7 = port C (0x02), pin 7 (0x07), active high (0x00)
        # RST = PC12 = port C (0x02), pin 12 (0x0c), active high (0x00)
        out.append(f'{indent}ili9341@1 {{\n')
        out.append(f'{indent}\tcompatible = "adafruit,yx240qv29";\n')
        out.append(f'{indent}\treg = <0x01>;\n')
        out.append(f'{indent}\tspi-max-frequency = <0x1e84800>;\n')
        out.append(f'{indent}\trotation = <0x5a>;\n')
        out.append(f'{indent}\tdc-gpios = <{pio_phandle} 0x02 0x07 0x00>;\n')
        out.append(f'{indent}\treset-gpios = <{pio_phandle} 0x02 0x0c 0x00>;\n')
        out.append(f'{indent}}};\n')
        replaced = True
        continue

    out.append(line)
    i += 1

if not replaced:
    print("ERROR: Could not find spidev@1 block", file=sys.stderr)
    sys.exit(1)

with open("/tmp/k2b-patched.dts", "w") as f:
    f.writelines(out)

print("Patched: replaced spidev@1 with ili9341@1")
