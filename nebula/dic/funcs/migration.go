package funcs

import (
	"github.com/cjxpj/nebula/dto"
)

func captureOutput(d *dto.DicInputs) (any, error) {
	if d.Inputs.LenOk(0) && d.Output != nil {
		return d.Output.Get(), nil
	}
	return "", nil
}

func interceptOutput(d *dto.DicInputs) (any, error) {
	if d.Inputs.LenOk(0) && d.Output != nil {
		res := d.Output.Get()
		d.Output.Clear()
		return res, nil
	}
	return "", nil
}

func stopProgram(d *dto.DicInputs) (any, error) {
	if d.Inputs.LenOk(0) {
		return nil, nil
	}
	return "", nil
}

func encodeDic(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return f.EncodeDic(), nil
}

func toUpper(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return f.ToUpper(), nil
}

func toLower(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return f.ToLower(), nil
}

func zipCompress(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return f.ZipFolder(), nil
}

func zipDecompress(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return f.UnZip(), nil
}

func dirSize(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return f.DirSize(), nil
}

func fileSize(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return f.FileSize(), nil
}

func fileRename(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return f.FileRename(), nil
}

func fileCopy(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return f.FileCopy(), nil
}

func doCount(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return f.Count(), nil
}

func doRandNum(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return f.RandNum(), nil
}

func randLetterUpperLower(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return f.RandLetter(0), nil
}

func randLetterUpper(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return f.RandLetter(1), nil
}

func randLetterLower(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return f.RandLetter(2), nil
}

func randLetterUpperLowerNum(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return f.RandLetter(3), nil
}

func randLetterLowerNum(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return f.RandLetter(4), nil
}

func randLetterUpperNum(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return f.RandLetter(5), nil
}

func randNumber(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return f.RandLetter(6), nil
}

func timestampFormattingTime(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return f.TimestampFormattingTime(), nil
}

func queryJson(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return f.QueryJson()
}

func isJson(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return f.IsJson(), nil
}

func jsonAdd(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return f.JsonAdd(), nil
}

func jsonAddString(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return f.JsonAddString(), nil
}

func jsonDelete(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return f.JsonDelete(), nil
}

func jsonIsKey(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return f.JsonIsKey(), nil
}

func jsonLen(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return f.JsonLen(), nil
}

func jsonPrettyPrint(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return f.JsonPrettyPrint()
}

func htmlParse(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return f.HtmlParse()
}

func htmlEncode(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return f.HtmlEncode()
}

func htmlDecode(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return f.HtmlDecode()
}

func enUtf8(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return f.EnUtf8(), nil
}

func deUtf8(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return f.DeUtf8(), nil
}

func getGif(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return f.GetGif(), nil
}

func drawImg(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return f.DrawImg(), nil
}

func doSort(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return f.Sort(), nil
}

func doRange(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return f.Range(), nil
}

func ed25519NewKeyFromSeed(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return f.Ed25519_NewKeyFromSeed()
}

func ed25519Sign(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return f.Ed25519_Sign()
}

func ed25519Verify(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return f.Ed25519_Verify()
}

func ed25519PublicKeyToCurve25519(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return f.Ed25519_PublicKeyToCurve25519()
}

func ed25519PrivateKeyToCurve25519(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return f.Ed25519_PrivateKeyToCurve25519()
}

func ed25519NewKeyFromCurve25519(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return f.Ed25519_NewKeyFromCurve25519()
}

func drawImgLoadFont(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return nil, f.DrawImgLoadFont()
}

func drawImgSetSize(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return nil, f.DrawImgSetSize()
}

func drawImgText(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return nil, f.DrawImgText()
}

func drawImgPoint(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return nil, f.DrawImgPoint()
}

func drawImgLine(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return nil, f.DrawImgLine()
}

func drawImgBrushLine(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return nil, f.DrawImgBrushLine()
}

func drawImgWaveLine(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return nil, f.DrawImgWaveLine()
}

func drawImgFloodFill(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return nil, f.DrawImgFloodFill()
}

func drawImgRectangleFill(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return nil, f.DrawImgRectangleFill()
}

func drawImgRectangleStroke(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return nil, f.DrawImgRectangleStroke()
}

func drawImgEllipseFill(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return nil, f.DrawImgEllipseFill()
}

func drawImgEllipse(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return nil, f.DrawImgEllipse()
}

func drawImgPieFill(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return nil, f.DrawImgPieFill()
}

func drawImgPie(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return nil, f.DrawImgPie()
}

func drawImgPolygon(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return nil, f.DrawImgPolygon()
}

func drawImgPolygons(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return nil, f.DrawImgPolygons()
}

func drawImgPaste(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return nil, f.DrawImgPaste()
}

func drawImgRotate(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return nil, f.DrawImgRotate()
}

func drawImgRoundCorners(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return nil, f.DrawImgRoundCorners()
}

func drawImgRandomDots(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return nil, f.DrawImgRandomDots()
}

func drawImgRandomLines(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return nil, f.DrawImgRandomLines()
}

func drawImgGrayscale(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return nil, f.DrawImgGrayscale()
}

func drawImgGaussianBlur(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return nil, f.DrawImgGaussianBlur()
}

func drawImgMosaic(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return nil, f.DrawImgMosaic()
}

func drawImgAllMosaic(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return nil, f.DrawImgAllMosaic()
}

func drawImgArc(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return nil, f.DrawImgArc()
}

func imageSimilarity(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return f.ImageSimilarity()
}

func gcCollect(d *dto.DicInputs) (any, error) {
	return nil, nil
}

func tencentGetApi(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return f.TencentGetApi()
}

func tencentGetApiCall(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return f.TencentGetApiCall()
}

func runCommandDecoder(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return f.RunCommandDecoder()
}

func runCommandVar(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return f.RunCommandVar()
}

func runCommandClose(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return f.RunCommandClose()
}

func runCommandInputText(d *dto.DicInputs) (any, error) {
	f := &DicFunc{
		Len:       d.Inputs.Len(),
		InputData: d.Inputs.List,
		Inputs:    d.Inputs,
	}
	return f.RunCommandInputText()
}
