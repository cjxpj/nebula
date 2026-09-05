package funcs

import (
	stdjson "encoding/json"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/cjxpj/nebula/dto"
)

// ============ 科学计算 ============

// 三角函数
func mathSin(d *dto.DicInputs) (any, error) { return math.Sin(d.Inputs.Float64(1)), nil }
func mathCos(d *dto.DicInputs) (any, error) { return math.Cos(d.Inputs.Float64(1)), nil }
func mathTan(d *dto.DicInputs) (any, error) { return math.Tan(d.Inputs.Float64(1)), nil }

// 反三角函数
func mathAsin(d *dto.DicInputs) (any, error) { return math.Asin(d.Inputs.Float64(1)), nil }
func mathAcos(d *dto.DicInputs) (any, error) { return math.Acos(d.Inputs.Float64(1)), nil }
func mathAtan(d *dto.DicInputs) (any, error) { return math.Atan(d.Inputs.Float64(1)), nil }

// atan2(y, x)：返回 y/x 的反正切，结果区间 [-π, π]，可区分象限。
func mathAtan2(d *dto.DicInputs) (any, error) {
	return math.Atan2(d.Inputs.Float64(1), d.Inputs.Float64(2)), nil
}

// 指数与对数
func mathPow(d *dto.DicInputs) (any, error) {
	return math.Pow(d.Inputs.Float64(1), d.Inputs.Float64(2)), nil
}
func mathExp(d *dto.DicInputs) (any, error)   { return math.Exp(d.Inputs.Float64(1)), nil }
func mathLog(d *dto.DicInputs) (any, error)   { return math.Log(d.Inputs.Float64(1)), nil }
func mathLog10(d *dto.DicInputs) (any, error) { return math.Log10(d.Inputs.Float64(1)), nil }
func mathLog2(d *dto.DicInputs) (any, error)  { return math.Log2(d.Inputs.Float64(1)), nil }
func mathSqrt(d *dto.DicInputs) (any, error)  { return math.Sqrt(d.Inputs.Float64(1)), nil }
func mathCbrt(d *dto.DicInputs) (any, error)  { return math.Cbrt(d.Inputs.Float64(1)), nil }

// 其他常用
func mathAbs(d *dto.DicInputs) (any, error)   { return math.Abs(d.Inputs.Float64(1)), nil }
func mathCeil(d *dto.DicInputs) (any, error)  { return math.Ceil(d.Inputs.Float64(1)), nil }
func mathFloor(d *dto.DicInputs) (any, error) { return math.Floor(d.Inputs.Float64(1)), nil }

// ============ 角度单位转换 ============
// 三角函数入参为弧度；测绘常用角度、百分度、密位，需先转弧度再计算。
const (
	degToRad = math.Pi / 180  // 度 → 弧度
	gonToRad = math.Pi / 200  // 百分度（gon）→ 弧度
	milToRad = math.Pi / 3200 // 密位（NATO mil）→ 弧度
)

func angDegToRad(d *dto.DicInputs) (any, error) { return d.Inputs.Float64(1) * degToRad, nil }
func angRadToDeg(d *dto.DicInputs) (any, error) { return d.Inputs.Float64(1) / degToRad, nil }
func angGonToRad(d *dto.DicInputs) (any, error) { return d.Inputs.Float64(1) * gonToRad, nil }
func angRadToGon(d *dto.DicInputs) (any, error) { return d.Inputs.Float64(1) / gonToRad, nil }
func angMilToRad(d *dto.DicInputs) (any, error) { return d.Inputs.Float64(1) * milToRad, nil }
func angRadToMil(d *dto.DicInputs) (any, error) { return d.Inputs.Float64(1) / milToRad, nil }

// 角度制三角函数：入参为度/百分度/密位，内部自动转弧度，调用意图更清晰。
func sinDeg(d *dto.DicInputs) (any, error) { return math.Sin(d.Inputs.Float64(1) * degToRad), nil }
func cosDeg(d *dto.DicInputs) (any, error) { return math.Cos(d.Inputs.Float64(1) * degToRad), nil }
func tanDeg(d *dto.DicInputs) (any, error) { return math.Tan(d.Inputs.Float64(1) * degToRad), nil }

func sinGon(d *dto.DicInputs) (any, error) { return math.Sin(d.Inputs.Float64(1) * gonToRad), nil }
func cosGon(d *dto.DicInputs) (any, error) { return math.Cos(d.Inputs.Float64(1) * gonToRad), nil }
func tanGon(d *dto.DicInputs) (any, error) { return math.Tan(d.Inputs.Float64(1) * gonToRad), nil }

func sinMil(d *dto.DicInputs) (any, error) { return math.Sin(d.Inputs.Float64(1) * milToRad), nil }
func cosMil(d *dto.DicInputs) (any, error) { return math.Cos(d.Inputs.Float64(1) * milToRad), nil }
func tanMil(d *dto.DicInputs) (any, error) { return math.Tan(d.Inputs.Float64(1) * milToRad), nil }

// ============ 统计 ============

// parseFloatList 将参数解析为浮点数切片。
// 仅一个参数且为 JSON 数组（如 [1,2,3]）时解析数组，否则把每个参数解析为浮点数。
func parseFloatList(d *dto.DicInputs) []float64 {
	if d.Inputs.Len() == 1 {
		s := strings.TrimSpace(d.Inputs.String(1))
		if strings.HasPrefix(s, "[") {
			var arr []float64
			if err := stdjson.Unmarshal([]byte(s), &arr); err == nil {
				return arr
			}
			var arrAny []any
			if err := stdjson.Unmarshal([]byte(s), &arrAny); err == nil {
				out := make([]float64, 0, len(arrAny))
				for _, v := range arrAny {
					switch vv := v.(type) {
					case float64:
						out = append(out, vv)
					case string:
						if f, err := strconv.ParseFloat(vv, 64); err == nil {
							out = append(out, f)
						}
					}
				}
				return out
			}
		}
	}
	out := make([]float64, 0, d.Inputs.Len())
	for i := 1; i <= d.Inputs.Len(); i++ {
		out = append(out, d.Inputs.Float64(i))
	}
	return out
}

// 求和
func statSum(d *dto.DicInputs) (any, error) {
	list := parseFloatList(d)
	var s float64
	for _, v := range list {
		s += v
	}
	return s, nil
}

// 计数
func statCount(d *dto.DicInputs) (any, error) {
	return len(parseFloatList(d)), nil
}

// 平均值
func statMean(d *dto.DicInputs) (any, error) {
	list := parseFloatList(d)
	if len(list) == 0 {
		return "", nil
	}
	var s float64
	for _, v := range list {
		s += v
	}
	return s / float64(len(list)), nil
}

// 中位数
func statMedian(d *dto.DicInputs) (any, error) {
	list := parseFloatList(d)
	if len(list) == 0 {
		return "", nil
	}
	sorted := make([]float64, len(list))
	copy(sorted, list)
	sort.Float64s(sorted)
	n := len(sorted)
	if n%2 == 1 {
		return sorted[n/2], nil
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2, nil
}

// 方差（总体方差，除以 N）
func statVariance(d *dto.DicInputs) (any, error) {
	list := parseFloatList(d)
	n := len(list)
	if n == 0 {
		return "", nil
	}
	var s float64
	for _, v := range list {
		s += v
	}
	mean := s / float64(n)
	var ss float64
	for _, v := range list {
		ss += (v - mean) * (v - mean)
	}
	return ss / float64(n), nil
}

// 标准差（总体标准差，除以 N）
func statStddev(d *dto.DicInputs) (any, error) {
	list := parseFloatList(d)
	n := len(list)
	if n == 0 {
		return "", nil
	}
	var s float64
	for _, v := range list {
		s += v
	}
	mean := s / float64(n)
	var ss float64
	for _, v := range list {
		ss += (v - mean) * (v - mean)
	}
	return math.Sqrt(ss / float64(n)), nil
}

// ============ 金融 ============

// 净现值 NPV(rate, cf1, cf2, ...)：现金流按 1/(1+rate)^t 折现求和（t 从 1 开始）。
func finNPV(d *dto.DicInputs) (any, error) {
	rate := d.Inputs.Float64(1)
	var npv float64
	for i := 2; i <= d.Inputs.Len(); i++ {
		npv += d.Inputs.Float64(i) / math.Pow(1+rate, float64(i-1))
	}
	return npv, nil
}

// 内部收益率 IRR：求使现金流（t 从 0 开始）净现值为 0 的折现率。
// 参数为现金流序列，首项通常为初始投入（负数）；无解时返回空串。
func finIRR(d *dto.DicInputs) (any, error) {
	cashflows := parseFloatList(d)
	if len(cashflows) < 2 {
		return "", nil
	}
	npv := func(r float64) float64 {
		var s float64
		for t, cf := range cashflows {
			s += cf / math.Pow(1+r, float64(t))
		}
		return s
	}

	lo, hi := -0.9999, 10.0
	flo, fhi := npv(lo), npv(hi)
	if flo == 0 {
		return lo, nil
	}
	if fhi == 0 {
		return hi, nil
	}
	// 区间端点同号，尝试扩大上限与负区间，仍无解则返回空。
	if flo*fhi > 0 {
		for _, r := range []float64{100.0, 1000.0, -0.5} {
			if f := npv(r); f == 0 {
				return r, nil
			} else if f*flo < 0 {
				hi, fhi = r, f
				break
			}
		}
		if flo*fhi > 0 {
			return "", nil
		}
	}

	for range 200 {
		mid := (lo + hi) / 2
		fm := npv(mid)
		if math.Abs(fm) < 1e-12 {
			return mid, nil
		}
		if flo*fm < 0 {
			hi, fhi = mid, fm
		} else {
			lo, flo = mid, fm
		}
	}
	return (lo + hi) / 2, nil
}

// ============ 复数（以实部、虚部两个参数表示） ============

func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// formatComplex 将复数的实部、虚部格式化为 a+bi 文本。
func formatComplex(re, im float64) string {
	if im == 0 {
		return formatFloat(re)
	}
	imStr := formatFloat(math.Abs(im))
	if imStr == "1" {
		imStr = ""
	}
	imPart := imStr + "i"
	if re == 0 {
		if im < 0 {
			return "-" + imPart
		}
		return imPart
	}
	if im < 0 {
		return formatFloat(re) + "-" + imPart
	}
	return formatFloat(re) + "+" + imPart
}

func complexArgs(d *dto.DicInputs) (complex128, complex128) {
	a := complex(d.Inputs.Float64(1), d.Inputs.Float64(2))
	b := complex(d.Inputs.Float64(3), d.Inputs.Float64(4))
	return a, b
}

func complexAdd(d *dto.DicInputs) (any, error) {
	a, b := complexArgs(d)
	c := a + b
	return formatComplex(real(c), imag(c)), nil
}

func complexSub(d *dto.DicInputs) (any, error) {
	a, b := complexArgs(d)
	c := a - b
	return formatComplex(real(c), imag(c)), nil
}

func complexMul(d *dto.DicInputs) (any, error) {
	a, b := complexArgs(d)
	c := a * b
	return formatComplex(real(c), imag(c)), nil
}

func complexDiv(d *dto.DicInputs) (any, error) {
	a, b := complexArgs(d)
	c := a / b
	return formatComplex(real(c), imag(c)), nil
}

// 复数模：返回复数的模（绝对值）。
func complexAbs(d *dto.DicInputs) (any, error) {
	return math.Hypot(d.Inputs.Float64(1), d.Inputs.Float64(2)), nil
}

// 复数共轭：实部不变、虚部取反。
func complexConj(d *dto.DicInputs) (any, error) {
	return formatComplex(d.Inputs.Float64(1), -d.Inputs.Float64(2)), nil
}

// ============ 矩阵（JSON 二维数组表示，如 [[1,2],[3,4]]） ============

type matrix [][]float64

func (m matrix) rows() int { return len(m) }
func (m matrix) cols() int {
	if len(m) == 0 {
		return 0
	}
	return len(m[0])
}

func parseMatrix(s string) (matrix, error) {
	var m [][]float64
	if err := stdjson.Unmarshal([]byte(s), &m); err != nil {
		return nil, err
	}
	return matrix(m), nil
}

func matrixJSON(m matrix) string {
	// 四舍五入到 12 位小数，消除浮点累计误差（如 0.6000000000000001 → 0.6）。
	out := make(matrix, len(m))
	for i, row := range m {
		out[i] = make([]float64, len(row))
		for j, v := range row {
			out[i][j] = math.Round(v*1e12) / 1e12
		}
	}
	b, err := stdjson.Marshal(out)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// 矩阵加：同型矩阵对应元素相加。
func matrixAdd(d *dto.DicInputs) (any, error) {
	a, err := parseMatrix(d.Inputs.String(1))
	if err != nil {
		return "", nil
	}
	b, err := parseMatrix(d.Inputs.String(2))
	if err != nil {
		return "", nil
	}
	if a.rows() != b.rows() || a.cols() != b.cols() || a.rows() == 0 || a.cols() == 0 {
		return "[]", nil
	}
	out := make(matrix, a.rows())
	for i := range a.rows() {
		out[i] = make([]float64, a.cols())
		for j := range a.cols() {
			out[i][j] = a[i][j] + b[i][j]
		}
	}
	return matrixJSON(out), nil
}

// 矩阵减：同型矩阵对应元素相减。
func matrixSub(d *dto.DicInputs) (any, error) {
	a, err := parseMatrix(d.Inputs.String(1))
	if err != nil {
		return "", nil
	}
	b, err := parseMatrix(d.Inputs.String(2))
	if err != nil {
		return "", nil
	}
	if a.rows() != b.rows() || a.cols() != b.cols() || a.rows() == 0 || a.cols() == 0 {
		return "[]", nil
	}
	out := make(matrix, a.rows())
	for i := range a.rows() {
		out[i] = make([]float64, a.cols())
		for j := range a.cols() {
			out[i][j] = a[i][j] - b[i][j]
		}
	}
	return matrixJSON(out), nil
}

// 矩阵乘：m×n 与 n×p 相乘得到 m×p。
func matrixMul(d *dto.DicInputs) (any, error) {
	a, err := parseMatrix(d.Inputs.String(1))
	if err != nil {
		return "", nil
	}
	b, err := parseMatrix(d.Inputs.String(2))
	if err != nil {
		return "", nil
	}
	if a.cols() != b.rows() || a.rows() == 0 || b.cols() == 0 {
		return "[]", nil
	}
	m, n, p := a.rows(), a.cols(), b.cols()
	out := make(matrix, m)
	for i := range m {
		out[i] = make([]float64, p)
		for j := range p {
			var s float64
			for k := range n {
				s += a[i][k] * b[k][j]
			}
			out[i][j] = s
		}
	}
	return matrixJSON(out), nil
}

// 矩阵转置。
func matrixTranspose(d *dto.DicInputs) (any, error) {
	a, err := parseMatrix(d.Inputs.String(1))
	if err != nil {
		return "", nil
	}
	if a.rows() == 0 {
		return "[]", nil
	}
	out := make(matrix, a.cols())
	for i := range a.cols() {
		out[i] = make([]float64, a.rows())
		for j := range a.rows() {
			out[i][j] = a[j][i]
		}
	}
	return matrixJSON(out), nil
}

// 矩阵行列式（高斯消元，非方阵返回空）。
func matrixDet(d *dto.DicInputs) (any, error) {
	a, err := parseMatrix(d.Inputs.String(1))
	if err != nil {
		return "", nil
	}
	n := a.rows()
	if n == 0 || a.cols() != n {
		return "", nil
	}
	// 复制矩阵，避免修改原数据
	m := make(matrix, n)
	for i := range n {
		m[i] = append([]float64(nil), a[i]...)
	}
	det := 1.0
	for i := range n {
		piv := i
		for j := i + 1; j < n; j++ {
			if math.Abs(m[j][i]) > math.Abs(m[piv][i]) {
				piv = j
			}
		}
		if m[piv][i] == 0 {
			return 0.0, nil
		}
		if piv != i {
			m[i], m[piv] = m[piv], m[i]
			det = -det
		}
		det *= m[i][i]
		for j := i + 1; j < n; j++ {
			f := m[j][i] / m[i][i]
			for k := i; k < n; k++ {
				m[j][k] -= f * m[i][k]
			}
		}
	}
	return det, nil
}

// 矩阵逆（高斯-约当消元，奇异或非方阵返回空）。
func matrixInverse(d *dto.DicInputs) (any, error) {
	a, err := parseMatrix(d.Inputs.String(1))
	if err != nil {
		return "", nil
	}
	n := a.rows()
	if n == 0 || a.cols() != n {
		return "", nil
	}
	// 增广矩阵 [A | I]
	aug := make(matrix, n)
	for i := range n {
		aug[i] = make([]float64, 2*n)
		for j := range n {
			aug[i][j] = a[i][j]
		}
		aug[i][n+i] = 1
	}
	for i := range n {
		piv := i
		for j := i + 1; j < n; j++ {
			if math.Abs(aug[j][i]) > math.Abs(aug[piv][i]) {
				piv = j
			}
		}
		if aug[piv][i] == 0 {
			return "", nil
		}
		if piv != i {
			aug[i], aug[piv] = aug[piv], aug[i]
		}
		div := aug[i][i]
		for k := range 2 * n {
			aug[i][k] /= div
		}
		for j := range n {
			if j == i {
				continue
			}
			f := aug[j][i]
			for k := range 2 * n {
				aug[j][k] -= f * aug[i][k]
			}
		}
	}
	inv := make(matrix, n)
	for i := range n {
		inv[i] = aug[i][n:]
	}
	return matrixJSON(inv), nil
}
