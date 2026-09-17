package drivers

// AvailabilityInput contains the driver availability change request.
type AvailabilityInput struct {
	DriverID  string
	Available bool
}

// Driver is the persisted driver availability state.
type Driver struct {
	ID        string
	Available bool
}

// DriverOutput is the public availability response.
type DriverOutput struct {
	ID        string `json:"id"`
	Available bool   `json:"available"`
}
