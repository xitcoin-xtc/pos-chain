package types

// ValidateAt rejects an otherwise valid attestation after its signed deadline.
func (a Attestation) ValidateAt(unixTime int64) error {
	if err := a.Validate(); err != nil {
		return err
	}
	if unixTime > a.DeadlineUnix {
		return ErrAttestationExpired
	}
	return nil
}
