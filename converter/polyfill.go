package converter

import (
	"bytes"
	"fmt"
	"regexp"
)

// FounderFitPolyfill 为方正飞腾（Founder FIT）发排 PS 提供缺少的前导字典与排版宏定义
const FounderFitPolyfill = `
% ====== Founder FIT Compatibility Polyfill ======
/fit_ps_dict where { pop } { /fit_ps_dict 1000 dict def } ifelse
/fit_graphics_dct where { pop } { /fit_graphics_dct 500 dict def } ifelse

fit_ps_dict begin
  /fitsetdef { } def
  /BP { } def
  /EP { showpage } def
  /psdefine { } def
  /FE {
    1 exch {
      1 index 1 index
      dup 16 (00) cvrs
      (G00) dup 1 4 -1 roll putinterval cvn
      put
      pop
    } for
  } bind def
  /tr { translate } bind def
  /sc { setcmykcolor } bind def
  /gs { gsave } bind def
  /gr { grestore } bind def
  /np { newpath } bind def
  /cp { closepath } bind def
  /m   { moveto } bind def
  /rm  { rmoveto } bind def
  /l   { lineto } bind def
  /rl  { rlineto } bind def
  /c   { curveto } bind def
  /sw  { show } bind def
  /ovprn? false def
  /patmatrix matrix def
  /RsvMt matrix def
  /RsvShdMt matrix def
  /Rect { /y2 exch def /x2 exch def /y1 exch def /x1 exch def x1 y1 moveto x2 y1 lineto x2 y2 lineto x1 y2 lineto closepath } bind def
  /zypoly {
    /poly_n exch def
    poly_n 1 ge {
      moveto
      poly_n 1 sub { lineto } repeat
      closepath
    } if
  } bind def
  /zyline {
    /poly_n exch def
    poly_n 1 ge {
      moveto
      poly_n 1 sub { lineto } repeat
    } if
  } bind def
  /zypath {
    /poly_n exch def
    poly_n 1 ge {
      moveto
      poly_n 1 sub { lineto } repeat
    } if
  } bind def
  /zycurve {
    /poly_n exch def
    poly_n 1 ge {
      moveto
      poly_n 1 sub { curveto } repeat
    } if
  } bind def
  /zyfill { fill } bind def
  /zystroke { stroke } bind def
  /zyclip { clip } bind def
  /zyrect { Rect } bind def
  /fsw { show } bind def
  /fxs { pop pop show } bind def
  /fys { pop pop show } bind def
  /VT  { pop pop show clear } bind def
  /SI {
    /Source exch def
    pop
    /height exch def
    /width exch def
    /bits exch def
    /decode exch def
    /bscan exch def
    ovprn? { /ovprnsav currentoverprint def true setoverprint } if
    <<
      /ImageType 1
      /Width width
      /Height height
      /BitsPerComponent bits
      /Decode decode
      /ImageMatrix bscan { [width 0 0 height 0 0] } { [width 0 0 height neg 0 height] } ifelse
      /DataSource /Source load
    >>
    bits 1 eq { imagemask } { image } ifelse
    ovprn? { ovprnsav setoverprint } if
  } bind def
  /errio { /bOpenFile false def } def
  /stshdclp { /shdclp exch def /RsvShdMt matrix currentmatrix def } bind def
  /shdeof   { gs RsvShdMt setmatrix shdclp eofill gr } bind def
  /shdf     { gs RsvShdMt setmatrix shdclp fill gr } bind def
  /rdstr_buf 65536 string def
  /rdstr {
    pop dup 65536 le {
      rdstr_buf 0 3 -1 roll getinterval
    } {
      string
    } ifelse
    currentfile exch readhexstring pop
  } bind def
end

/rdstr_buf where { pop } { /rdstr_buf 65536 string def } ifelse
/rdstr where { pop } {
  /rdstr {
    pop dup 65536 le {
      rdstr_buf 0 3 -1 roll getinterval
    } {
      string
    } ifelse
    currentfile exch readhexstring pop
  } bind def
} ifelse

/fitsetdef where { pop } { /fitsetdef { } def } ifelse
/BP where { pop } { /BP { } def } ifelse
/EP where { pop } { /EP { showpage } def } ifelse
/psdefine where { pop } { /psdefine { } def } ifelse
/FE where { pop } {
  /FE {
    1 exch {
      1 index 1 index
      dup 16 (00) cvrs
      (G00) dup 1 4 -1 roll putinterval cvn
      put
      pop
    } for
  } bind def
} ifelse
/tr where { pop } { /tr { translate } bind def } ifelse
/sc where { pop } { /sc { setcmykcolor } bind def } ifelse
/gs where { pop } { /gs { gsave } bind def } ifelse
/gr where { pop } { /gr { grestore } bind def } ifelse
/np where { pop } { /np { newpath } bind def } ifelse
/cp where { pop } { /cp { closepath } bind def } ifelse
/m   where { pop } { /m   { moveto } bind def } ifelse
/rm  where { pop } { /rm  { rmoveto } bind def } ifelse
/l   where { pop } { /l   { lineto } bind def } ifelse
/rl  where { pop } { /rl  { rlineto } bind def } ifelse
/c   where { pop } { /c   { curveto } bind def } ifelse
/sw  where { pop } { /sw  { show } bind def } ifelse
/ovprn? where { pop } { /ovprn? false def } ifelse
/RsvShdMt where { pop } { /RsvShdMt matrix def } ifelse
/stshdclp where { pop } { /stshdclp { /shdclp exch def /RsvShdMt matrix currentmatrix def } bind def } ifelse
/shdeof where { pop } { /shdeof { gs RsvShdMt setmatrix shdclp eofill gr } bind def } ifelse
/shdf where { pop } { /shdf { gs RsvShdMt setmatrix shdclp fill gr } bind def } ifelse
/Rect where { pop } { /Rect { /y2 exch def /x2 exch def /y1 exch def /x1 exch def x1 y1 moveto x2 y1 lineto x2 y2 lineto x1 y2 lineto closepath } bind def } ifelse
/zypoly where { pop } {
  /zypoly {
    /poly_n exch def
    poly_n 1 ge {
      moveto
      poly_n 1 sub { lineto } repeat
      closepath
    } if
  } bind def
} ifelse
/zyline where { pop } {
  /zyline {
    /poly_n exch def
    poly_n 1 ge {
      moveto
      poly_n 1 sub { lineto } repeat
    } if
  } bind def
} ifelse
/zypath where { pop } {
  /zypath {
    /poly_n exch def
    poly_n 1 ge {
      moveto
      poly_n 1 sub { lineto } repeat
    } if
  } bind def
} ifelse
/zycurve where { pop } {
  /zycurve {
    /poly_n exch def
    poly_n 1 ge {
      moveto
      poly_n 1 sub { curveto } repeat
    } if
  } bind def
} ifelse
/zyfill where { pop } { /zyfill { fill } bind def } ifelse
/zystroke where { pop } { /zystroke { stroke } bind def } ifelse
/zyclip where { pop } { /zyclip { clip } bind def } ifelse
/zyrect where { pop } { /zyrect { Rect } bind def } ifelse
/fsw where { pop } { /fsw { show } bind def } ifelse
/fxs where { pop } { /fxs { pop pop show } bind def } ifelse
/fys where { pop } { /fys { pop pop show } bind def } ifelse
/VT  where { pop } { /VT  { pop pop show clear } bind def } ifelse
/SI where { pop } {
  /SI {
    /Source exch def
    pop
    /height exch def
    /width exch def
    /bits exch def
    /decode exch def
    /bscan exch def
    ovprn? { /ovprnsav currentoverprint def true setoverprint } if
    <<
      /ImageType 1
      /Width width
      /Height height
      /BitsPerComponent bits
      /Decode decode
      /ImageMatrix bscan { [width 0 0 height 0 0] } { [width 0 0 height neg 0 height] } ifelse
      /DataSource /Source load
    >>
    bits 1 eq { imagemask } { image } ifelse
    ovprn? { ovprnsav setoverprint } if
  } bind def
} ifelse
/errio where { pop } { /errio { /bOpenFile false def } def } ifelse

fit_graphics_dct begin
  /stshdclp { /shdclp exch def /RsvShdMt matrix currentmatrix def } bind def
  /shdeof   { gs RsvShdMt setmatrix shdclp eofill gr } bind def
  /shdf     { gs RsvShdMt setmatrix shdclp fill gr } bind def
  /ovprn?   false def
  /shdsv    0 def
  /sc       { setcmykcolor } bind def
  /gs       { gsave } bind def
  /gr       { grestore } bind def
  /m        { moveto } bind def
  /l        { lineto } bind def
  /zypoly {
    /poly_n exch def
    poly_n 1 ge {
      moveto
      poly_n 1 sub { lineto } repeat
      closepath
    } if
  } bind def
end
% ================================================
`

// InjectFounderPolyfill 在 PS 数据头部安全注入方正飞腾发排环境兼容垫片及中文字体合成定义
func InjectFounderPolyfill(psData []byte) []byte {
	// 检查是否包含方正飞腾特征或私有字典调用
	isFounder := bytes.Contains(psData, []byte("fit_ps_dict")) ||
		bytes.Contains(psData, []byte("Founder")) ||
		bytes.Contains(psData, []byte("FIT")) ||
		bytes.Contains(psData, []byte("fitsetdef"))

	if !isFounder {
		return psData
	}

	// 扫描 PS 中引用的所有方正双字节字体（如 /FZBSK--GBK1-0, /FZHTK--GBK1-0）
	reFounderFont := regexp.MustCompile(`/([A-Za-z0-9_\-]+)--(GBK1-0|GB-EUC-H)`)
	matches := reFounderFont.FindAllSubmatch(psData, -1)
	fontComposes := make(map[string]bool)
	var composeBuf bytes.Buffer
	composeBuf.WriteString("\n% ====== Founder CJK Font Composition ======\n")
	for _, m := range matches {
		prefix := string(m[1])
		suffix := string(m[2])
		key := prefix + "--" + suffix
		if !fontComposes[key] {
			fontComposes[key] = true
			composeBuf.WriteString(fmt.Sprintf("/%s where { pop } { /%s /CIDFallBack def } ifelse\n", prefix, prefix))
			composeBuf.WriteString(fmt.Sprintf("/%s--%s /GBK-EUC-H [/%s] composefont pop\n", prefix, suffix, prefix))
		}
	}
	composeBuf.WriteString("% ==========================================\n")

	polyfillBytes := append([]byte(FounderFitPolyfill+"\n"), composeBuf.Bytes()...)

	// 优先在首行（例如 %!PS-Adobe-3.0）之后插入，符合 DSC 规范
	firstLF := bytes.IndexByte(psData, '\n')
	if firstLF != -1 && bytes.HasPrefix(psData, []byte("%!")) {
		var out bytes.Buffer
		out.Grow(len(psData) + len(polyfillBytes))
		out.Write(psData[:firstLF+1])
		out.Write(polyfillBytes)
		out.Write(psData[firstLF+1:])
		return out.Bytes()
	}

	// 否则直接前置
	var out bytes.Buffer
	out.Grow(len(psData) + len(polyfillBytes))
	out.Write(polyfillBytes)
	out.Write(psData)
	return out.Bytes()
}
