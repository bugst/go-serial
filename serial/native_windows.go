package serial

/*

// MSDN article on Serial Communications:
// http://msdn.microsoft.com/en-us/library/ff802693.aspx

// Arduino Playground article on serial communication with Windows API:
// http://playground.arduino.cc/Interfacing/CPPWindows

#include <stdlib.h>
#include <windows.h>

*/
import "C"
import "syscall"

// OS dependent values

const devFolder = ""
const regexFilter = "(ttyS|ttyUSB|ttyACM|ttyAMA|rfcomm|ttyO)[0-9]{1,3}"

// opaque type that implements SerialPort interface for linux
type windowsSerialPort struct {
	Handle int
}

func GetPortsList() ([]string, error) {
	return nil, nil
	/*
	   private static String[] getWindowsPortNames(Pattern pattern, Comparator<String> comparator) {
	       String[] portNames = serialInterface.getSerialPortNames();
	       if(portNames == null){
	           return new String[]{};
	       }
	       TreeSet<String> ports = new TreeSet<String>(comparator);
	       for(String portName : portNames){
	           if(pattern.matcher(portName).find()){
	               ports.add(portName);
	           }
	       }
	       return ports.toArray(new String[ports.size()]);
	   }
	*/
}

func (port *windowsSerialPort) Close() error {
	return nil
}

func (port *windowsSerialPort) Read(p []byte) (n int, err error) {
	return syscall.Read(port.Handle, p)
}

func (port *windowsSerialPort) Write(p []byte) (n int, err error) {
	return syscall.Write(port.Handle, p)
}

func OpenPort(portName string, useTIOCEXCL bool) (SerialPort, error) {
	portName = "\\\\.\\" + portName

	handle, err := syscall.CreateFile(portName, syscall.GENERIC_READ|syscall.GENERIC_WRITE, 0, nil, syscall.OPEN_EXISTING, syscall.FILE_FLAG_OVERLAPPED, 0)
	//handle := C.CreateFile(C.CString(portName), C.GENERIC_READ | C.GENERIC_WRITE, 0, 0, C.OPEN_EXISTING, C.FILE_FLAG_OVERLAPPED, 0)

	/*
	   JNIEXPORT jlong JNICALL Java_jssc_SerialNativeInterface_openPort(JNIEnv *env, jobject object, jstring portName, jboolean useTIOCEXCL){
	       char prefix[] = "\\\\.\\";
	       const char* port = env->GetStringUTFChars(portName, JNI_FALSE);

	       //since 2.1.0 -> string concat fix
	       char portFullName[strlen(prefix) + strlen(port) + 1];
	       strcpy(portFullName, prefix);
	       strcat(portFullName, port);
	       //<- since 2.1.0

	       HANDLE hComm = CreateFile(portFullName,
	                          	   	  GENERIC_READ | GENERIC_WRITE,
	                          	   	  0,
	                          	   	  0,
	                          	   	  OPEN_EXISTING,
	                          	   	  FILE_FLAG_OVERLAPPED,
	                          	   	  0);
	       env->ReleaseStringUTFChars(portName, port);
	*/
	/*
		if handle != syscall.INVALID_HANDLE_VALUE {
			var dcb C.DCB
			if C.GetCommState(handle, &dcb) != 0 {
				C.CloseHandle(handle)
				return nil,
			}
		}
	*/
	/*    //since 2.3.0 ->
	    if(hComm != INVALID_HANDLE_VALUE){
	    	DCB *dcb = new DCB();
	    	if(!GetCommState(hComm, dcb)){
	    		CloseHandle(hComm);//since 2.7.0
	    		hComm = (HANDLE)jssc_SerialNativeInterface_ERR_INCORRECT_SERIAL_PORT;//(-4)Incorrect serial port
	    	}
	    	delete dcb;
	    }
	    else {
	    	DWORD errorValue = GetLastError();
	    	if(errorValue == ERROR_ACCESS_DENIED){
	    		hComm = (HANDLE)jssc_SerialNativeInterface_ERR_PORT_BUSY;//(-1)Port busy
	    	}
	    	else if(errorValue == ERROR_FILE_NOT_FOUND){
	    		hComm = (HANDLE)jssc_SerialNativeInterface_ERR_PORT_NOT_FOUND;//(-2)Port not found
	    	}
	    }
	    //<- since 2.3.0
	    return (jlong)hComm;//since 2.4.0 changed to jlong
	};

	*/
}

// vi:ts=2
