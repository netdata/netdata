use super::*;
use std::io::Write;
use std::net::IpAddr;
use std::ops::Range;

pub(super) struct JournalBufWriter<'a> {
    data: &'a mut Vec<u8>,
    refs: &'a mut Vec<Range<usize>>,
    value_starts: &'a mut Vec<usize>,
    ibuf: itoa::Buffer,
}

impl<'a> JournalBufWriter<'a> {
    pub(super) fn new(
        data: &'a mut Vec<u8>,
        refs: &'a mut Vec<Range<usize>>,
        value_starts: &'a mut Vec<usize>,
    ) -> Self {
        Self {
            data,
            refs,
            value_starts,
            ibuf: itoa::Buffer::new(),
        }
    }

    pub(super) fn push_str(&mut self, name: &str, value: &str) {
        if !value.is_empty() {
            let start = self.data.len();
            self.data.extend_from_slice(name.as_bytes());
            self.data.push(b'=');
            let value_start = self.data.len();
            self.data.extend_from_slice(value.as_bytes());
            self.finish_field(start, value_start);
        }
    }

    pub(super) fn push_u8(&mut self, name: &str, value: u8) {
        if value != 0 {
            self.push_number(name, value as u64);
        }
    }

    pub(super) fn push_u16(&mut self, name: &str, value: u16) {
        if value != 0 {
            self.push_number(name, value as u64);
        }
    }

    pub(super) fn push_u32(&mut self, name: &str, value: u32) {
        if value != 0 {
            self.push_number(name, value as u64);
        }
    }

    pub(super) fn push_u64(&mut self, name: &str, value: u64) {
        if value != 0 {
            self.push_number(name, value);
        }
    }

    pub(super) fn push_u8_when(&mut self, present: bool, name: &str, value: u8) {
        if present {
            self.push_number(name, value as u64);
        }
    }

    pub(super) fn push_u16_when(&mut self, present: bool, name: &str, value: u16) {
        if present {
            self.push_number(name, value as u64);
        }
    }

    pub(super) fn push_u64_when(&mut self, present: bool, name: &str, value: u64) {
        if present {
            self.push_number(name, value);
        }
    }

    pub(super) fn push_opt_ip(&mut self, name: &str, value: Option<IpAddr>) {
        if let Some(ip) = value {
            let start = self.data.len();
            self.data.extend_from_slice(name.as_bytes());
            self.data.push(b'=');
            let value_start = self.data.len();
            let _ = write!(self.data, "{}", ip);
            self.finish_field(start, value_start);
        }
    }

    pub(super) fn push_direction(&mut self, present: bool, value: &str) {
        if present && value != DIRECTION_UNDEFINED {
            let start = self.data.len();
            self.data.extend_from_slice(b"DIRECTION=");
            let value_start = self.data.len();
            self.data.extend_from_slice(value.as_bytes());
            self.finish_field(start, value_start);
        }
    }

    pub(super) fn push_prefix(&mut self, name: &str, value: Option<IpAddr>, mask: u8) {
        if let Some(ip) = value {
            let start = self.data.len();
            self.data.extend_from_slice(name.as_bytes());
            self.data.push(b'=');
            let value_start = self.data.len();
            let _ = write!(self.data, "{}", ip);
            if mask > 0 {
                self.data.push(b'/');
                self.data
                    .extend_from_slice(self.ibuf.format(mask as u64).as_bytes());
            }
            self.finish_field(start, value_start);
        }
    }

    pub(super) fn push_mac(&mut self, name: &str, value: [u8; 6]) {
        if value != [0u8; 6] {
            let start = self.data.len();
            self.data.extend_from_slice(name.as_bytes());
            self.data.push(b'=');
            let value_start = self.data.len();
            let _ = write!(
                self.data,
                "{:02x}:{:02x}:{:02x}:{:02x}:{:02x}:{:02x}",
                value[0], value[1], value[2], value[3], value[4], value[5]
            );
            self.finish_field(start, value_start);
        }
    }

    fn push_number(&mut self, name: &str, value: u64) {
        let start = self.data.len();
        self.data.extend_from_slice(name.as_bytes());
        self.data.push(b'=');
        let value_start = self.data.len();
        self.data
            .extend_from_slice(self.ibuf.format(value).as_bytes());
        self.finish_field(start, value_start);
    }

    fn finish_field(&mut self, start: usize, value_start: usize) {
        self.refs.push(start..self.data.len());
        self.value_starts.push(value_start);
    }
}
