package models

import (
	"encoding/xml"
	"fmt"
	"strconv"
)

// ConditionalUint is a custom type that can be either a uint64 or a bool.
// It implements the xml.MarshalerAttr and xml.UnmarshalerAttr interfaces
// to allow for conditional XML attributes in the form of either a number or a boolean.
// This is useful for cases where you want to represent a value that can be
// either a numeric value or a boolean flag in XML serialization.
type ConditionalUint struct {
	U *uint64
	B *bool
}

// MarshalXMLAttr implements the xml.MarshalerAttr interface.
// It serializes the ConditionalUint to an XML attribute.
// If the uint64 value is set, it will be serialized as a number.
// If the bool value is set, it will be serialized as a boolean.
// If neither value is set, it will return an empty xml.Attr.
// This allows for conditional attributes in XML, where the attribute
// may or may not be present based on the value of the ConditionalUint.
// If both values are set, the uint64 value takes precedence.
// This is useful for cases where you want to represent a value that can be
// either a numeric value or a boolean flag in XML serialization.
// The name parameter is the name of the XML attribute to be serialized.
func (c ConditionalUint) MarshalXMLAttr(name xml.Name) (xml.Attr, error) {
	if c.U != nil {
		return xml.Attr{Name: name, Value: strconv.FormatUint(*c.U, 10)}, nil
	}

	if c.B != nil {
		return xml.Attr{Name: name, Value: strconv.FormatBool(*c.B)}, nil
	}

	return xml.Attr{}, nil
}

// UnmarshalXMLAttr implements the xml.UnmarshalerAttr interface.
// It deserializes the XML attribute value into the ConditionalUint.
func (c *ConditionalUint) UnmarshalXMLAttr(attr xml.Attr) error {
	u, err := strconv.ParseUint(attr.Value, 10, 64)
	if err == nil {
		c.U = &u
		return nil
	}

	b, err := strconv.ParseBool(attr.Value)
	if err == nil {
		c.B = &b
		return nil
	}

	return fmt.Errorf("ConditionalUint: can't UnmarshalXMLAttr %#v", attr)
}

var (
	_ xml.MarshalerAttr   = ConditionalUint{}
	_ xml.UnmarshalerAttr = &ConditionalUint{}
)
