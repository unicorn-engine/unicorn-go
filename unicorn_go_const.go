package unicorn

type GoUcError int

func (u GoUcError) Error() string {
	return u.String()
}

const (
	UCGO_ERR_REG_BATCH_MALLOC GoUcError = -1 // Error in uc_reg_..._batch_helper's malloc()
)
