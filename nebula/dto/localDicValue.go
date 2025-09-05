package dto

func (d *LocalDicValue) ForGetRun() any {
	return d.For.Run
}

func (d *LocalDicValue) ForEachGetRun() any {
	return d.ForEach.Run
}
