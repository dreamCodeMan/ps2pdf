#!/usr/bin/env python3
"""
ps2pdf.py  --  将方正 PS 文件转换为可搜索/可复制的 PDF

用法:
    python3 ps2pdf.py <input.ps> [output.pdf]

示例:
    python3 ps2pdf.py 3390-1.ps
    python3 ps2pdf.py 3390-2.ps 3390-2.pdf

特性:
    - 通用自动探测 PS 中引用的全部图片，智能匹配并直接使用本地真实绝对路径（无需任何软链接）
    - 自动修复 PS 中的 Windows 绝对路径，跨平台无缝适配
    - 禁用 /psdefine 反盗版检查
    - 注入 ToUnicode CMap，使 PDF 文字可正常搜索、复制
    - 修正 DLF-32769 标点字体宽度（消除标点后多余空格）
    - 修正 CMYK JPEG 颜色反相（报头徽标等）
    - 零中间文件污染：临时文件在系统隔离目录中自动流转并自动销毁
    - 需要系统安装 Ghostscript (gs) 和 Python 包 pikepdf
"""

import os
import re
import sys
import shutil
import tempfile
import subprocess
import pikepdf


def find_gs():
    """查找系统中 Ghostscript 的可执行路径"""
    if sys.platform == 'win32':
        for name in ['gswin64c', 'gswin32c', 'gs']:
            which = shutil.which(name)
            if which:
                return which
        import glob
        for pf in [os.environ.get('ProgramFiles', 'C:\\Program Files'),
                   os.environ.get('ProgramFiles(x86)', 'C:\\Program Files (x86)')]:
            matches = glob.glob(os.path.join(pf, 'gs', 'gs*', 'bin', 'gswin*c.exe'))
            if matches:
                return matches[-1]
        return 'gswin64c'

    for p in ['/opt/homebrew/bin/gs', '/usr/local/bin/gs']:
        if os.path.isfile(p) and os.access(p, os.X_OK):
            return p
    which_gs = shutil.which('gs')
    return which_gs if which_gs else 'gs'


def normalize_filename(name):
    """归一化文件名（全半角标点、连续点、空格与小写），用于智能模糊匹配"""
    name = name.lower()
    name = name.replace('（', '(').replace('）', ')')
    name = name.replace('【', '[').replace('】', ']')
    name = name.replace('—', '-').replace('..', '.')
    return re.sub(r'\s+', '', name)


def build_local_file_index(workspace):
    """建立工作区及其子目录的本地文件检索索引"""
    exact_map = {}
    lower_map = {}
    norm_map = {}
    all_files = []

    for root, dirs, files in os.walk(workspace):
        # 忽略隐藏目录
        dirs[:] = [d for d in dirs if not d.startswith('.')]
        for f in files:
            full_path = os.path.join(root, f)
            # 排除符号链接
            if os.path.islink(full_path):
                continue
            all_files.append(full_path)
            if f not in exact_map:
                exact_map[f] = full_path
                lower_map[f.lower()] = full_path
                norm_map[normalize_filename(f)] = full_path

    return exact_map, lower_map, norm_map, all_files


def decode_bytes(data):
    """解码 GBK 字节串，失败则退回 Latin-1"""
    try:
        return data.decode('gbk')
    except Exception:
        return data.decode('latin1', errors='replace')


def patch_ps_direct(ps_data, workspace):
    """
    修补 PS 文件：
    1. 禁用 /psdefine 反盗版宏
    2. 自动探测所有图片路径引用，智能模糊匹配本地实际文件，直接替换为真实绝对路径（彻底避免符号链接）
    """
    # 1. 禁用 /psdefine 反盗版检查
    idx1 = ps_data.find(b'/psdefine')
    if idx1 != -1:
        idx2 = ps_data.find(b'} bd', idx1) + 4
        ps_data = ps_data[:idx1] + b'/psdefine { } bd' + ps_data[idx2:]
        print("  已修补 psdefine（禁用反盗版）")

    # 2. 建立本地文件索引
    exact_map, lower_map, norm_map, all_files = build_local_file_index(workspace)

    # 3. 扫描 PS 文件中引用的所有图片路径 (匹配括号内的图片路径)
    pattern = rb'\(([^()\r\n]+?\.(?:jpg|jpeg|png|tif|tiff|eps|bmp))\)'
    matches = re.findall(pattern, ps_data, re.IGNORECASE)

    seen_raw = set()
    replacements = []

    for raw_path in matches:
        if raw_path in seen_raw:
            continue
        seen_raw.add(raw_path)

        # 截取文件名部分
        last_slash = max(raw_path.rfind(b'\\\\'), raw_path.rfind(b'\\'), raw_path.rfind(b'/'))
        raw_filename = raw_path[last_slash+1:] if last_slash != -1 else raw_path
        raw_filename = raw_filename.lstrip(b'\\/')

        target_name = decode_bytes(raw_filename)

        # 多级智能模糊匹配
        real_file_path = None
        if target_name in exact_map:
            real_file_path = exact_map[target_name]
        elif target_name.lower() in lower_map:
            real_file_path = lower_map[target_name.lower()]
        elif normalize_filename(target_name) in norm_map:
            real_file_path = norm_map[normalize_filename(target_name)]
        else:
            target_norm = normalize_filename(target_name)
            for p in all_files:
                base = os.path.basename(p)
                if normalize_filename(base) == target_norm:
                    real_file_path = p
                    break

        if not real_file_path:
            print(f"  [警告] 未在本地找到匹配图片: {target_name} (PS路径: {decode_bytes(raw_path)})")
            continue

        replacements.append((raw_path, real_file_path.encode('utf-8'), os.path.basename(real_file_path)))

    # 4. 直接替换为本地真实绝对路径
    for raw_path, real_path_bytes, file_name in replacements:
        cnt = ps_data.count(raw_path)
        ps_data = ps_data.replace(raw_path, real_path_bytes)
        print(f"  [直接路径] {file_name} ({cnt}处) -> {real_path_bytes.decode('utf-8')}")

    return ps_data


def parse_ps_string(s_bytes):
    """解析 PostScript 字符串转义"""
    res = bytearray()
    i = 0
    n = len(s_bytes)
    while i < n:
        b = s_bytes[i:i+1]
        if b == b'\\':
            i += 1
            if i >= n: break
            c = s_bytes[i:i+1]
            if c in b'01234567':
                octal = c
                if i+1 < n and s_bytes[i+1:i+2] in b'01234567':
                    i += 1; octal += s_bytes[i:i+1]
                    if i+1 < n and s_bytes[i+1:i+2] in b'01234567':
                        i += 1; octal += s_bytes[i:i+1]
                res.append(int(octal, 8))
            elif c == b'n':  res.append(10)
            elif c == b'r':  res.append(13)
            elif c == b't':  res.append(9)
            elif c == b'(':  res.append(ord('('))
            elif c == b')':  res.append(ord(')'))
            elif c == b'\\': res.append(ord('\\'))
            else:             res.append(c[0])
        else:
            res.append(b[0])
        i += 1
    return bytes(res)


def make_to_unicode_cmap(code_to_char):
    """生成符合 Adobe 规范的 ToUnicode CMap"""
    lines = [
        "/CIDInit /ProcSet findresource begin",
        "12 dict begin", "begincmap",
        "/CIDSystemInfo <<",
        "  /Registry (Adobe)", "  /Ordering (UCS)", "  /Supplement 0",
        ">> def",
        "/CMapName /Founder-ToUnicode def",
        "/CMapType 2 def",
        "1 begincodespacerange", "  <00> <FF>", "endcodespacerange",
    ]
    items = sorted(code_to_char.items())
    for i in range(0, len(items), 100):
        chunk = items[i:i+100]
        lines.append(f"{len(chunk)} beginbfchar")
        for code, ch in chunk:
            uni_hex = ch.encode('utf-16-be').hex().upper()
            lines.append(f"<{code:02X}> <{uni_hex}>")
        lines.append("endbfchar")
    lines += ["endcmap", "CMapName currentdict /CMap defineresource pop", "end", "end"]
    return "\n".join(lines).encode('ascii')


def build_char_mapping(ps_lines):
    """从 PS 文件中提取 DownLoadCode 映射"""
    current_font = None
    mapping = {}
    last_dl_code = None

    for line in ps_lines:
        if b'setfont' in line or b'selectfont' in line:
            m = re.search(rb'(DLF-[0-9]+-[0-9]+-[0-9]+)', line)
            if m:
                current_font = m.group(1).decode('ascii')

        if line.startswith(b'%%DownLoadCode'):
            val = line[14:].strip(b'\r\n')
            if val.startswith(b' '): val = val[1:]
            if not val:
                last_dl_code = ' '
            else:
                try:    last_dl_code = val.decode('gbk')
                except: last_dl_code = val.decode('latin1', errors='replace')

        m_match = re.search(rb'\((.*?)\)\s*\[[^\]]*\]\s*\d+\s*(?:fxs|fys|VT)', line)
        if m_match and current_font and last_dl_code:
            raw    = m_match.group(1)
            parsed = parse_ps_string(raw)
            if len(parsed) == 1:
                code = parsed[0]
                mapping.setdefault(current_font, {})[code] = last_dl_code
            last_dl_code = None

    return mapping


def main():
    if len(sys.argv) < 2:
        print(__doc__)
        sys.exit(1)

    ps_path   = os.path.abspath(sys.argv[1])
    workspace = os.path.dirname(ps_path)
    stem      = os.path.splitext(os.path.basename(ps_path))[0]

    final_pdf = os.path.abspath(sys.argv[2]) if len(sys.argv) >= 3 \
                else os.path.join(workspace, stem + '.pdf')

    print(f"\n=== 构建: {os.path.basename(ps_path)} -> {os.path.basename(final_pdf)} ===\n")

    # 使用系统临时目录流转中间文件，处理完毕自动彻底清理，工作区 0 污染
    with tempfile.TemporaryDirectory(prefix="build_pdf_") as tmp_dir:
        base_pdf = os.path.join(tmp_dir, stem + '_base.pdf')
        fixed_ps = os.path.join(tmp_dir, stem + '_fixed.ps')

        print("[1/4] 自动扫描图片并直接重定向到本地真实路径...")
        with open(ps_path, 'rb') as f:
            ps_data = f.read()
        ps_data = patch_ps_direct(ps_data, workspace)
        with open(fixed_ps, 'wb') as f:
            f.write(ps_data)

        print("[2/4] Ghostscript 渲染基础 PDF...")
        gs_bin = find_gs()
        gs_cmd = [
            gs_bin, '-dBATCH', '-dNOPAUSE', '-dNOSAFER',
            '-sDEVICE=pdfwrite',
            '-dDEVICEWIDTHPOINTS=1114.02',
            '-dDEVICEHEIGHTPOINTS=1545.93',
            '-dFIXEDMEDIA',
            f'-sOutputFile={base_pdf}',
            fixed_ps,
        ]
        run_kwargs = {}
        if sys.platform == 'win32':
            # 隐藏 Windows 下运行 Ghostscript 时弹出的 CMD 控制台黑窗口
            run_kwargs['creationflags'] = getattr(subprocess, 'CREATE_NO_WINDOW', 0x08000000)
        res = subprocess.run(gs_cmd, capture_output=True, text=True, **run_kwargs)
        if res.returncode != 0:
            print("GS 错误:\n", res.stdout[-1000:])
            sys.exit(1)
        for line in res.stdout.splitlines():
            if 'Warning' in line or 'Substitut' in line or "Can't" in line:
                print("  [GS]", line)

        print("[3/4] 解析字符映射...")
        with open(ps_path, 'rb') as f:
            ps_lines = f.readlines()
        mapping = build_char_mapping(ps_lines)
        total   = sum(len(v) for v in mapping.values())
        print(f"  {len(mapping)} 种字体，共 {total} 个字符映射")

        print("[4/4] 注入 ToUnicode / 修正字宽 / 修正 CMYK 并生成最终文件...")
        pdf  = pikepdf.open(base_pdf)
        page = pdf.pages[0]

        injected = 0
        for alias, f_obj in page.Resources.Font.items():
            base_font  = str(f_obj.get('/BaseFont', ''))
            clean_name = base_font.split('+')[-1].lstrip('/')
            if clean_name in mapping:
                f_obj['/ToUnicode'] = pikepdf.Stream(pdf, make_to_unicode_cmap(mapping[clean_name]))
                injected += 1
            if 'DLF-32769' in clean_name and '/Widths' in f_obj:
                w = f_obj['/Widths']
                for i in range(len(w)): w[i] = 1000

        print(f"  ToUnicode 注入: {injected} 个字体")

        xobjs = page.Resources.get('/XObject', {})
        for name, obj in xobjs.items():
            if str(obj.get('/ColorSpace', '')) == '/DeviceCMYK':
                obj['/Decode'] = pikepdf.Array([1, 0, 1, 0, 1, 0, 1, 0])
                print(f"  CMYK 颜色已修正: XObject {name}")

        pdf.save(final_pdf)
        size_kb = os.path.getsize(final_pdf) // 1024
        print(f"\n✓ 完成！{final_pdf}  ({size_kb} KB)\n")


if __name__ == '__main__':
    main()
