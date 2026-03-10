#!/usr/bin/env python3
# Unicorn Engine
# By Dang Hoang Vu, 2013
import sys, re, os, argparse

INCLUDE_FILES = [ 'arm.h', 'arm64.h', 'mips.h', 'x86.h', 'sparc.h', 'm68k.h', 'ppc.h', 'riscv.h', 's390x.h', 'tricore.h', 'unicorn.h' ]

go_template = {
    'header': "package unicorn\n// For Unicorn Engine. AUTO-GENERATED FILE, DO NOT EDIT [%s_const.go]\nconst (\n",
    'footer': ")",
    'line_format': '\t%s = %s\n',
    'out_file': '%s_const.go',
    # prefixes for constant filenames of all archs - case sensitive
    'arm.h': 'arm',
    'arm64.h': 'arm64',
    'mips.h': 'mips',
    'x86.h': 'x86',
    'sparc.h': 'sparc',
    'm68k.h': 'm68k',
    'ppc.h': 'ppc',
    'riscv.h': 'riscv',
    's390x.h' : 's390x',
    'tricore.h' : 'tricore',
    'unicorn.h': 'unicorn',
    'comment_open': '//',
    'comment_close': '',
}

# markup for comments to be added to autogen files
MARKUP = '//>'

def gen(include_dir: str, output_dir: str):
    global INCLUDE_FILES
    templ = go_template
    for target in INCLUDE_FILES:
        prefix = templ[target]
        outfn = os.path.join(output_dir, templ['out_file'] % prefix)
        outfile = open(outfn + ".tmp", 'wb')   # open as binary prevents windows newlines
        outfile.write((templ['header'] % prefix).encode("utf-8"))
        if target == 'unicorn.h':
            prefix = ''
        with open(os.path.join(include_dir, target)) as f:
            lines = f.readlines()

        previous = {}
        count = 0
        skip = 0
        in_comment = False
        
        for lno, line in enumerate(lines):
            if "/*" in line:
                in_comment = True
            if "*/" in line:
                in_comment = False
            if in_comment:
                continue
            if skip > 0:
                # Due to clang-format, values may come up in the next line
                skip -= 1
                continue
            line = line.strip()

            if line.startswith(MARKUP):  # markup for comments
                outfile.write(("\n%s%s%s\n" %(templ['comment_open'], \
                            line.replace(MARKUP, ''), templ['comment_close'])).encode("utf-8"))
                continue

            if line == '' or line.startswith('//'):
                continue

            tmp = line.strip().split(',')
            if len(tmp) >= 2 and tmp[0] != "#define" and not tmp[0].startswith("UC_"):
                continue
            for t in tmp:
                t = t.strip()
                if not t or t.startswith('//'): continue
                f = re.split('\\s+', t)

                # parse #define UC_TARGET (num)
                define = False
                if f[0] == '#define' and len(f) >= 3:
                    define = True
                    f.pop(0)
                    f.insert(1, '=')
                if f[0].startswith("UC_" + prefix.upper()) or f[0].startswith("UC_CPU"):
                    if len(f) > 1 and f[1] not in ('//', '='):
                        print("WARNING: Unable to convert %s" % f)
                        print("  Line =", line)
                        continue
                    elif len(f) > 1 and f[1] == '=':
                        # Like:
                        # UC_A = 
                        #       (1 << 2)
                        # #define UC_B \
                        #              (UC_A | UC_C)
                        # Let's search the next line
                        if len(f) == 2:
                            if lno == len(lines) - 1:
                                print("WARNING: Unable to convert %s" % f)
                                print("  Line =", line)
                                continue
                            skip += 1
                            next_line = lines[lno + 1]
                            next_line_tmp = next_line.strip().split(",")
                            rhs = next_line_tmp[0]
                        elif f[-1] == "\\":
                            idx = 0
                            rhs = ""
                            while True:
                                idx += 1
                                if lno + idx == len(lines):
                                    print("WARNING: Unable to convert %s" % f)
                                    print("  Line =", line)
                                    continue
                                skip += 1
                                next_line = lines[lno + idx]
                                next_line_f = re.split('\\s+', next_line.strip())
                                if next_line_f[-1] == "\\":
                                    rhs += "".join(next_line_f[:-1])
                                else:
                                    rhs += next_line.strip()
                                    break
                        else:
                            rhs = ''.join(f[2:])
                    else:
                        rhs = str(count)

                    
                    lhs = f[0].strip()
                    #print(f'lhs: {lhs} rhs: {rhs} f:{f}')
                    # evaluate bitshifts in constants e.g. "UC_X86 = 1 << 1"
                    match = re.match(r'(?P<rhs>\s*\d+\s*<<\s*\d+\s*)', rhs)
                    if match:
                        rhs = str(eval(match.group(1)))
                    else:
                        # evaluate references to other constants e.g. "UC_ARM_REG_X = UC_ARM_REG_SP"
                        match = re.match(r'^([^\d]\w+)$', rhs)
                        if match:
                            rhs = previous[match.group(1)]

                    if not rhs.isdigit():
                        for k, v in previous.items():
                            rhs = re.sub(r'\b%s\b' % k, v, rhs)
                        rhs = str(eval(rhs))

                    lhs_strip = re.sub(r'^UC_', '', lhs)
                    count = int(rhs) + 1
                    if (count == 1):
                        outfile.write(("\n").encode("utf-8"))

                    outfile.write((templ['line_format'] % (lhs_strip, rhs)).encode("utf-8"))
                    previous[lhs] = str(rhs)

        outfile.write((templ['footer']).encode("utf-8"))
        outfile.close()

        if os.path.isfile(outfn):
            with open(outfn, "rb") as infile:
                cur_data = infile.read()
            with open(outfn + ".tmp", "rb") as infile:
                new_data = infile.read()
            if cur_data == new_data:
                os.unlink(outfn + ".tmp")
            else:
                os.unlink(outfn)
                os.rename(outfn + ".tmp", outfn)
        else:
            os.rename(outfn + ".tmp", outfn)

def create_destination_dir(output):
    if not os.path.isdir(output):
        os.makedirs(output, exist_ok=True)

def parse_arguments():
    parser = argparse.ArgumentParser(formatter_class=argparse.RawDescriptionHelpFormatter,
                description=f"""
{sys.argv[0]} generates golang constant files from unicorn C sources.
If one of the options are not submitted, the program will fallback to legacy values:
  -i {os.path.join('..', 'include', 'unicorn')}
  -o ./go/unicorn/
                            """)

    parser.add_argument("-i", "--includes",  default=os.path.join('..', 'include', 'unicorn'),  help="Input directory containing unicorn headers")
    parser.add_argument("-o", "--output", default="./go/unicorn/",  help="Output directory")
    args = parser.parse_args()

    return args

def main():
    args = parse_arguments()

    if not os.path.isdir(args.includes):
        print(f"includes directory does not exists : {args.includes}")
        exit(1)
    if not os.path.isdir(args.output): # ensure includes directory exists before creating output
        os.makedirs(args.output, exist_ok=True)
    
    gen(args.includes, args.output)

if __name__ == "__main__":
    main()
