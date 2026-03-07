package state

import "encoding/json"

func (s *Store) SetParam(module, key string, value any) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	s.Set(ParamKey(module, key), b)
	return nil
}

func (s *Store) GetParam(module, key string, out any) bool {
	raw, ok := s.Get(ParamKey(module, key))
	if !ok {
		return false
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return false
	}
	return true
}
