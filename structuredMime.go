package rfc2822

import (
	"fmt"
	"net/mail"
	"time"
)

/*
	Example:
	To: =?ISO-8859-1?Q?Keld_J=F8rn_Simonsen?= (Keld) <用户@例子.广告>
	will become
	{
		Name: Keld Jørn Simonsen (Keld)
		Address: <用户@例子.广告>
		AddressText: =?utf-8?b?S2VsZCBKw7hybiBTaW1vbnNlbiAoS2VsZCk=?= <用户@例子.广告>
	}
*/
type Address struct {
	Name        string // Utf-8 string
	Address     string // Utf-8/ASCII string
	AddressText string // Encoded Address format
}

type FormattedRootHeaders struct {
	Headers     map[string][]string
	BadHeaders  map[string][]string
	Subject     string
	References  []string
	From        []Address
	To          []Address
	Cc          []Address
	Bcc         []Address
	Sender      []Address
	ReplyTo     []Address
	DeliveredTo []Address
	ReturnPath  []Address
	Priority    string
	MessageID   string
	InReplyTo   []string
	Date        time.Time
	ContentType ContentType
}

func NewFormattedRootHeaders() FormattedRootHeaders {
	return FormattedRootHeaders{
		Headers:     map[string][]string{},
		BadHeaders:  map[string][]string{},
		Subject:     "",
		References:  []string{},
		From:        []Address{},
		To:          []Address{},
		Cc:          []Address{},
		Bcc:         []Address{},
		Sender:      []Address{},
		ReplyTo:     []Address{},
		DeliveredTo: []Address{},
		ReturnPath:  []Address{},
		Priority:    "",
		MessageID:   "",
		InReplyTo:   []string{},
		Date:        time.Time{},
	}
}

func GetRootHeaderCallback(sm *FormattedRootHeaders) func(node *Node) error {
	return func(node *Node) error {
		parsedHeaders := node.ParsedHeader

		sm.ContentType = node.ContentType
		// Don't process bad headers, let lib consumer deal with it
		sm.BadHeaders = node.BadHeaders

		for k, v := range parsedHeaders {
			switch k {
			case "subject":
				// can be encode-word, needs decoding
				// example Subject: =?UTF-8?B?0LDQvdC00YA=?=
				if len(v) != 0 {
					// If there were repeated Subject header fields
					// choose the last one
					sm.Subject = decodeToUTF8Base64Header(v[len(v)-1])
				} else {
					sm.Subject = ""
				}
			case "date":
				// date type
				var decodedDate time.Time
				var decodeErr error
				if len(v) != 0 {
					dateString := v[len(v)-1]
					decodedDate, decodeErr = mail.ParseDate(dateString)
					if decodeErr != nil {
						return fmt.Errorf("Unable to parse date %v", dateString)
					}
				} else {
					decodedDate = time.Now()
				}
				sm.Date = decodedDate
			// Note: Message-Id can not have rfc2047 encoded words
			case "references":
				// references array
				var referencesArr []string
				for _, refs := range v {
					res, err := MsgIDList(refs)
					if err != nil {
						return fmt.Errorf("Unable to parse references %v: %v", refs, err)
					}
					referencesArr = append(referencesArr, res...)
				}
				sm.References = append(sm.References, referencesArr...)
			case "message-id":
				// A message should have only one message ID
				// if more than one then maybe show parsing error
				if len(v) == 0 {
					return fmt.Errorf("No message-id header")
				}

				if len(v) > 1 {
					return fmt.Errorf("Can't have more than one message-id header")
				}

				res, err := MsgIDList(v[0])
				if err != nil {
					return fmt.Errorf("Unable to parse message-id %v", v[0], err)
				}

				sm.MessageID = res[0]

			case "in-reply-to":
				var irts []string
				for _, refs := range v {
					res, err := MsgIDList(refs)
					if err != nil {
						return fmt.Errorf("Unable to parse references %v: %v", refs, err)
					}
					irts = append(irts, res...)
				}
				sm.InReplyTo = append(sm.InReplyTo, irts...)
			case "priority", "x-priority", "x-msmail-priority", "importance":
				// Priority parser
				// Could be a number like "1" or a string "High"
				// Right now keeping the raw string, maybe add a parser later
				sm.Priority = v[len(v)-1] // Use the latest header if there were more than one
			case "to", "from", "cc", "bcc", "sender", "reply-to", "delivered-to", "return-path":
				// UTF8 email addresses according to the RFCs 5890, 5891 and 5892 are left in unicode
				// they are not parsed into puny-code.
				var parsedAddresses []*mail.Address
				var parseError error
				var a Address
				for _, addr := range v {
					parsedAddresses, parseError = parseAddress(addr)
					if parseError != nil {
						return fmt.Errorf("Error parsing address header: %v, %v", addr, parseError)
					}
					for _, parsedAddr := range parsedAddresses {
						a.Name = parsedAddr.Name
						a.Address = parsedAddr.Address
						a.Name = parsedAddr.String()

						if k == "from" {
							sm.From = append(sm.From, a)
						} else if k == "to" {
							sm.To = append(sm.To, a)
						} else if k == "cc" {
							sm.Cc = append(sm.Cc, a)
						} else if k == "bcc" {
							sm.Bcc = append(sm.Bcc, a)
						} else if k == "sender" {
							sm.Sender = append(sm.Sender, a)
						} else if k == "reply-to" {
							sm.ReplyTo = append(sm.ReplyTo, a)
						} else if k == "delivered-to" {
							sm.DeliveredTo = append(sm.DeliveredTo, a)
						} else if k == "return-path" {
							sm.ReturnPath = append(sm.ReturnPath, a)
						}
					}
				}
			default:
				// put it in the headers thing
				sm.Headers[k] = v
			}
		}

		//Validations:

		// TODO:
		// Must have messageId

		if sm.MessageID == "" {
			return fmt.Errorf("Message-ID header can not be empty")
		}

		// https://datatracker.ietf.org/doc/html/rfc5322#section-3.6.2
		if len(sm.From) == 0 && len(sm.Sender) == 0 {
			return fmt.Errorf("From and Sender headers both can not be 0")
		}
		/*
			If the from field contains more than one mailbox specification
			in the mailbox-list, then the sender field, containing the field name "Sender" and a
			single mailbox specification, MUST appear in the message.
		*/
		if len(sm.From) > 1 {
			if len(sm.Sender) != 1 {
				return fmt.Errorf("Sender header is neeed when there are multiple From values")
			}
		}

		return nil
	}
}
